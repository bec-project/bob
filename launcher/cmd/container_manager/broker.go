package main

// One bob per user, started by the user's first GUI and kept while any client holds a lease.
//
// The broker (`--broker`) runs one Manager for the user. A client finds it with
// `bob-manager [--config bob.toml]`, which makes sure the broker runs (starting a detached one
// when its socket does not answer) and prints {"socket": "<path>"}. The client connects to that
// socket and sends one JSON line, {"consumer": "<name>", "pid": <pid>, "config": "<absolute
// bob.toml>"}. The broker starts bob for the first client and answers every client with
// {"connection": {...}, "config": "<its bob.toml>", "consumers": <n>} or {"error": "<text>"}
// once bob is up. A client holds its lease for as long as it keeps the connection open, so a
// client that crashes ends its lease with it. A client can also send {"stop": true, "reason":
// "<text>"} instead, to stop bob for every client (to start it again with another bob.toml, say).
// When bob stops while clients hold it, each gets {"stopped": "<why>"} before its lease closes. The linger time after the last lease ends, the
// broker stops bob and exits. It also checks that bob still runs: once its container stopped or
// is gone, the broker closes every lease, cleans up and exits, so the next client starts a new bob.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"bob/container_manager"
)

const (
	brokerSocket = "broker.sock"
	brokerLock   = "broker.lock"
	brokerLog    = "broker.log"
	// defaultLinger keeps bob through a client restart, such as reopening the GUI.
	defaultLinger = 90 * time.Second
	// clientDeadline covers a broker that is still removing the previous container.
	clientDeadline = 3 * time.Minute
	helloTimeout   = 10 * time.Second
)

// firstClientWait bounds a broker whose starting client never connected; tests shorten it.
var firstClientWait = 30 * time.Second

// healthInterval is how often the broker checks that bob still runs; tests shorten it.
var healthInterval = 5 * time.Second

const (
	healthTimeout = 10 * time.Second
	// healthFailures checks in a row that the engine could not answer count as bob gone: with no
	// engine (Docker Desktop quit, say) its containers are gone too.
	healthFailures = 6
)

// lifecycle is the part of containermanager.Manager the broker uses.
type lifecycle interface {
	Start(ctx context.Context, options containermanager.Options) (containermanager.Connection, error)
	// Check returns nil while bob runs, an error wrapping containermanager.ErrStopped once it
	// does not, and another error when the engine could not tell.
	Check(ctx context.Context) error
	Stop(ctx context.Context) error
}

type hello struct {
	Consumer string `json:"consumer"`
	PID      int    `json:"pid"`
	Config   string `json:"config"`
	// Stop asks the broker to stop bob for every client and exit; Reason tells the others why.
	Stop   bool   `json:"stop,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type brokerReply struct {
	Connection *containermanager.Connection `json:"connection,omitempty"`
	Config     string                       `json:"config,omitempty"`
	Consumers  int                          `json:"consumers,omitempty"`
	Error      string                       `json:"error,omitempty"`
	// Stopped says why bob stopped, to clients that held it, and to the client that stopped it.
	Stopped string `json:"stopped,omitempty"`
}

// runtimeDir is the private per-user directory holding the broker's socket, lock and log.
// It must be on a local filesystem (home directories on NFS do not carry Unix sockets).
func runtimeDir() (string, error) {
	directory := os.Getenv("BOB_RUNTIME_DIR")
	switch {
	case directory != "":
	case os.Getenv("XDG_RUNTIME_DIR") != "":
		directory = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "bob")
	case runtime.GOOS == "darwin" && os.Getenv("TMPDIR") != "":
		// macOS gives every user a private TMPDIR, the same for GUI apps and terminals.
		directory = filepath.Join(os.Getenv("TMPDIR"), "bob")
	default:
		directory = filepath.Join(os.TempDir(), fmt.Sprintf("bob-%d", os.Getuid()))
	}
	// The broker could not listen on a longer socket path; say so instead of retrying.
	if socket := filepath.Join(directory, brokerSocket); len(socket) > maxSocketPath() {
		return "", fmt.Errorf("%s is too long for a Unix socket (%d bytes, at most %d); set BOB_RUNTIME_DIR to a shorter directory",
			socket, len(socket), maxSocketPath())
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(stat.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s must be a directory only you can access", directory)
	}
	return directory, nil
}

// maxSocketPath is the longest Unix socket path the platform accepts: sun_path less its NUL.
func maxSocketPath() int {
	if runtime.GOOS == "linux" {
		return 107
	}
	return 103
}

// broker serves one bob to every client that connects, and stops it after the last one.
type broker struct {
	manager lifecycle
	options containermanager.Options
	config  string
	linger  time.Duration
	log     io.Writer

	startCtx   context.Context
	ready      chan struct{} // closed when Start returned
	connection containermanager.Connection
	startErr   error
	watched    chan struct{} // closed when the health watch ended; nil when bob did not start

	mu        sync.Mutex
	started   bool // Start was called, by the first client
	consumers int
	open      map[net.Conn]struct{}
	idle      *time.Timer
	stop      chan struct{}
	stopOnce  sync.Once
	// why bob stops, for the clients that hold it; written before stop closes
	stopReason string
}

func newBroker(manager lifecycle, options containermanager.Options, config string, linger time.Duration, log io.Writer) *broker {
	b := &broker{
		manager: manager, options: options, config: config, linger: linger, log: log,
		ready: make(chan struct{}), open: map[net.Conn]struct{}{}, stop: make(chan struct{}),
	}
	b.idle = time.AfterFunc(max(firstClientWait, linger), b.shutdownIfIdle)
	return b
}

func (b *broker) logf(format string, args ...any) {
	fmt.Fprintf(b.log, "%s bob broker: %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// requestStop stops the broker once; reason, when not empty, tells the clients that hold bob why.
func (b *broker) requestStop(reason string) {
	b.stopOnce.Do(func() {
		b.stopReason = reason
		close(b.stop)
	})
}

func (b *broker) shutdownIfIdle() {
	// Under the lock, so a client either joins first or sees the broker stopping.
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consumers == 0 {
		b.logf("no client left; stopping")
		b.requestStop("")
	}
}

// start runs bob for the first client; every client waits for ready.
func (b *broker) start() {
	b.logf("starting bob with %s", b.config)
	b.connection, b.startErr = b.manager.Start(b.startCtx, b.options)
	if b.startErr != nil {
		b.logf("bob did not start: %v", b.startErr)
	} else {
		b.logf("bob is up as %s at %s", b.connection.Container, b.connection.URL)
		b.watched = make(chan struct{})
		go b.watch(b.watched)
	}
	close(b.ready)
	if b.startErr != nil {
		// clients already waiting get the error; then there is nothing to serve
		time.AfterFunc(time.Second, func() { b.requestStop("") })
	}
}

// watch checks that bob still runs until the broker stops, and stops the broker once bob is gone:
// its clients then see their leases end, and the next client starts a new bob.
func (b *broker) watch(done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(b.startCtx, healthTimeout)
		err := b.manager.Check(ctx)
		cancel()
		reason := ""
		switch {
		case err == nil:
			failures = 0
			continue
		case errors.Is(err, containermanager.ErrStopped):
			b.logf("%v; ending every lease", err)
			reason = err.Error()
		default:
			failures++
			b.logf("cannot check bob (%d of %d): %v", failures, healthFailures, err)
			if failures < healthFailures {
				continue
			}
			b.logf("the engine did not answer %d checks in a row; ending every lease", failures)
			reason = "bob stopped: the container engine stopped answering"
		}
		b.requestStop(reason)
		return
	}
}

// serve runs the accept loop until the last lease lapses or ctx ends, then stops bob if a
// client made the broker start it.
func (b *broker) serve(ctx context.Context, listener net.Listener) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	b.startCtx = startCtx
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go b.handle(conn)
		}
	}()
	select {
	case <-b.stop:
	case <-ctx.Done():
		b.logf("interrupted; stopping")
		b.requestStop("bob stopped: its broker was interrupted")
	}
	b.idle.Stop()
	_ = listener.Close()
	b.mu.Lock()
	for conn := range b.open {
		if b.stopReason != "" {
			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			_ = json.NewEncoder(conn).Encode(brokerReply{Stopped: b.stopReason})
		}
		_ = conn.Close()
	}
	// A client that has not counted itself by now sees the stop and starts nothing.
	started := b.started
	b.mu.Unlock()
	if !started {
		return nil
	}
	// Calls to one Manager are serialized: wait for Start and the last Check before Stop.
	cancelStart()
	<-b.ready
	if b.watched != nil {
		<-b.watched
	}
	err := b.manager.Stop(context.Background())
	if err != nil {
		b.logf("cleanup failed: %v", err)
		return fmt.Errorf("cleanup: %w", err)
	}
	b.logf("bob stopped")
	return nil
}

func (b *broker) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(helloTimeout))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	var greeting hello
	if err != nil || json.Unmarshal(line, &greeting) != nil {
		return // `bob-manager` checking that the broker answers, or not a client
	}
	_ = conn.SetReadDeadline(time.Time{})
	if greeting.Stop {
		reason := greeting.Reason
		if reason == "" {
			reason = fmt.Sprintf("bob was stopped by %s", greeting.Consumer)
		}
		b.logf("client %q (pid %d) stops bob: %s", greeting.Consumer, greeting.PID, reason)
		b.requestStop(reason)
		_ = json.NewEncoder(conn).Encode(brokerReply{Stopped: reason})
		return
	}

	b.mu.Lock()
	select {
	case <-b.stop:
		b.mu.Unlock()
		_ = json.NewEncoder(conn).Encode(brokerReply{Error: "bob is shutting down; try again"})
		return
	default:
	}
	b.consumers++
	b.open[conn] = struct{}{}
	b.idle.Stop()
	if !b.started {
		b.started = true
		go b.start()
	}
	count := b.consumers
	b.mu.Unlock()
	b.logf("client %q (pid %d) joined; %d holding bob", greeting.Consumer, greeting.PID, count)

	defer func() {
		b.mu.Lock()
		delete(b.open, conn)
		b.consumers--
		left := b.consumers
		if left == 0 {
			b.idle.Reset(b.linger)
		}
		b.mu.Unlock()
		b.logf("client %q (pid %d) left; %d holding bob", greeting.Consumer, greeting.PID, left)
	}()

	select {
	case <-b.ready:
	case <-b.stop:
		return
	}
	select {
	case <-b.stop: // both may be ready at once; a stopping broker serves nobody
		return
	default:
	}
	reply := brokerReply{Config: b.config, Consumers: count}
	if b.startErr != nil {
		reply.Error = b.startErr.Error()
	} else {
		connection := b.connection
		reply.Connection = &connection
	}
	if err := json.NewEncoder(conn).Encode(reply); err != nil || reply.Error != "" {
		return
	}
	// The lease lasts until the client closes the connection (or its process ends).
	_, _ = io.Copy(io.Discard, reader)
}

// runBroker becomes the user's broker unless another one holds the lock, then serves bob.
func runBroker(ctx context.Context, config string, options containermanager.Options, linger time.Duration) error {
	directory, err := runtimeDir()
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(directory, brokerLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil // another broker serves this user
		}
		return err
	}
	socket := filepath.Join(directory, brokerSocket)
	_ = os.Remove(socket) // left by a broker that died; the lock proves it is gone
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = listener.Close()
		return err
	}
	return newBroker(&containermanager.Manager{}, options, config, linger, os.Stderr).serve(ctx, listener)
}

// spawnBroker starts a detached broker for config and does not wait for it.
func spawnBroker(directory, config string, linger time.Duration) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(directory, brokerLog), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(executable, "--broker", "--config", config, "--linger", linger.String())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, logFile, logFile
	// Its own session: closing the GUI or terminal that asked for it must not reach the broker.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap it if it exits while this process waits
	return nil
}

// ensureBroker returns the user's broker socket once a broker answers on it, starting one when
// none does.
func ensureBroker(ctx context.Context, directory, config string, linger time.Duration) (string, error) {
	socket := filepath.Join(directory, brokerSocket)
	deadline := time.Now().Add(clientDeadline)
	var lastSpawn time.Time
	for {
		conn, err := net.DialTimeout("unix", socket, 2*time.Second)
		if err == nil {
			_ = conn.Close() // no hello: the broker counts no lease for this check
			return socket, nil
		}
		// A broker that is still stopping the previous bob holds the lock, so a new one exits
		// at once: try again until the old one is done.
		if time.Since(lastSpawn) > 3*time.Second {
			if err := spawnBroker(directory, config, linger); err != nil {
				return "", fmt.Errorf("start the bob broker: %w", err)
			}
			lastSpawn = time.Now()
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no bob broker answered on %s; see %s", socket, filepath.Join(directory, brokerLog))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// runLocate makes sure the user's broker runs and prints its socket for a client.
func runLocate(ctx context.Context, config string, linger time.Duration, out io.Writer) error {
	directory, err := runtimeDir()
	if err != nil {
		return err
	}
	socket, err := ensureBroker(ctx, directory, config, linger)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	return json.NewEncoder(out).Encode(map[string]string{"socket": socket})
}
