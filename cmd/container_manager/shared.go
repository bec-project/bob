package main

// Shared mode: one bob per user, kept while any consumer holds a lease.
//
// `--shared` makes this process a consumer. It connects to the user's broker socket, starting
// a broker first when none answers, prints the broker's connection line like the classic mode
// does, and holds its lease until it is interrupted, its parent process exits, or the broker
// goes away. The broker (`--broker`, started detached by the first consumer) runs one Manager.
// It replies to every consumer with the same connection and tears bob down once the last
// lease has been gone for the linger time.
//
// Protocol on the socket, one JSON object per line: the consumer sends
// {"consumer": "<name>", "pid": <pid>, "config": "<absolute bob.toml>"}; the broker answers
// {"connection": {...}, "config": "<its bob.toml>", "consumers": <n>} or {"error": "<text>"}
// once bob is up. The lease lasts as long as the consumer keeps the connection open.

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
	// defaultLinger keeps bob through a consumer restart, such as reopening the GUI.
	defaultLinger = 90 * time.Second
	// firstConsumerWait bounds a broker whose starting consumer never connected.
	firstConsumerWait = 30 * time.Second
	// consumerDeadline covers bob's own 90 s startup limit and a broker that is still
	// removing the previous container.
	consumerDeadline = 3 * time.Minute
	helloTimeout     = 10 * time.Second
)

// lifecycle is the part of containermanager.Manager the broker uses.
type lifecycle interface {
	Start(ctx context.Context, options containermanager.Options) (containermanager.Connection, error)
	Stop(ctx context.Context) error
}

type hello struct {
	Consumer string `json:"consumer"`
	PID      int    `json:"pid"`
	Config   string `json:"config"`
}

type brokerReply struct {
	Connection *containermanager.Connection `json:"connection,omitempty"`
	Config     string                       `json:"config,omitempty"`
	Consumers  int                          `json:"consumers,omitempty"`
	Error      string                       `json:"error,omitempty"`
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

// broker serves one bob to every consumer that connects, and stops it after the last one.
type broker struct {
	manager lifecycle
	options containermanager.Options
	config  string
	linger  time.Duration
	log     io.Writer

	ready      chan struct{} // closed when Start returned
	connection containermanager.Connection
	startErr   error

	mu        sync.Mutex
	consumers int
	open      map[net.Conn]struct{}
	idle      *time.Timer
	stop      chan struct{}
	stopOnce  sync.Once
}

func newBroker(manager lifecycle, options containermanager.Options, config string, linger time.Duration, log io.Writer) *broker {
	b := &broker{
		manager: manager, options: options, config: config, linger: linger, log: log,
		ready: make(chan struct{}), open: map[net.Conn]struct{}{}, stop: make(chan struct{}),
	}
	b.idle = time.AfterFunc(max(firstConsumerWait, linger), b.shutdownIfIdle)
	return b
}

func (b *broker) logf(format string, args ...any) {
	fmt.Fprintf(b.log, "%s bob broker: %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func (b *broker) requestStop() {
	b.stopOnce.Do(func() { close(b.stop) })
}

func (b *broker) shutdownIfIdle() {
	// Under the lock, so a consumer either joins first or sees the broker stopping.
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consumers == 0 {
		b.logf("no consumer left; stopping bob")
		b.requestStop()
	}
}

// serve runs bob and the accept loop until the last lease lapses or ctx ends, then stops bob.
func (b *broker) serve(ctx context.Context, listener net.Listener) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	go func() {
		b.logf("starting bob with %s", b.config)
		b.connection, b.startErr = b.manager.Start(startCtx, b.options)
		if b.startErr != nil {
			b.logf("bob did not start: %v", b.startErr)
		} else {
			b.logf("bob is up as %s at %s", b.connection.Container, b.connection.URL)
		}
		close(b.ready)
		if b.startErr != nil {
			// consumers already waiting get the error; then there is nothing to serve
			time.AfterFunc(time.Second, b.requestStop)
		}
	}()
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
		b.logf("interrupted; stopping bob")
		b.requestStop()
	}
	b.idle.Stop()
	_ = listener.Close()
	b.mu.Lock()
	for conn := range b.open {
		_ = conn.Close()
	}
	b.mu.Unlock()
	// Calls to one Manager are serialized: wait for Start before Stop.
	cancelStart()
	<-b.ready
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
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

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
	count := b.consumers
	b.mu.Unlock()
	b.logf("consumer %q (pid %d) joined; %d holding bob", greeting.Consumer, greeting.PID, count)

	defer func() {
		b.mu.Lock()
		delete(b.open, conn)
		b.consumers--
		left := b.consumers
		if left == 0 {
			b.idle.Reset(b.linger)
		}
		b.mu.Unlock()
		b.logf("consumer %q (pid %d) left; %d holding bob", greeting.Consumer, greeting.PID, left)
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
	// The lease lasts until the consumer closes the connection (or its process ends).
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
	// Its own session: a terminal's Ctrl-C for one consumer must not reach the shared bob.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap it if it exits while this consumer runs
	return nil
}

// dialBroker connects to the user's broker, starting one when none answers.
func dialBroker(ctx context.Context, directory, config string, linger time.Duration) (net.Conn, error) {
	socket := filepath.Join(directory, brokerSocket)
	deadline := time.Now().Add(consumerDeadline)
	var lastSpawn time.Time
	for {
		conn, err := net.DialTimeout("unix", socket, 2*time.Second)
		if err == nil {
			return conn, nil
		}
		// A broker that is still stopping the previous bob holds the lock, so a new one exits
		// at once: try again until the old one is done.
		if time.Since(lastSpawn) > 3*time.Second {
			if err := spawnBroker(directory, config, linger); err != nil {
				return nil, fmt.Errorf("start the bob broker: %w", err)
			}
			lastSpawn = time.Now()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("no bob broker answered on %s; see %s", socket, filepath.Join(directory, brokerLog))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// lease asks the broker for bob and returns the open connection that holds the lease.
func lease(ctx context.Context, conn net.Conn, consumer, config string) (brokerReply, *bufio.Reader, error) {
	greeting := hello{Consumer: consumer, PID: os.Getpid(), Config: config}
	if err := json.NewEncoder(conn).Encode(greeting); err != nil {
		return brokerReply{}, nil, err
	}
	reader := bufio.NewReader(conn)
	replies := make(chan error, 1)
	var reply brokerReply
	go func() {
		line, err := reader.ReadBytes('\n')
		if err == nil {
			err = json.Unmarshal(line, &reply)
		}
		replies <- err
	}()
	select {
	case err := <-replies:
		if err != nil {
			return reply, nil, fmt.Errorf("the bob broker closed the connection: %w", err)
		}
	case <-ctx.Done():
		return reply, nil, ctx.Err()
	}
	if reply.Error != "" {
		return reply, nil, errors.New(reply.Error)
	}
	if reply.Connection == nil {
		return reply, nil, errors.New("the bob broker sent no connection")
	}
	return reply, reader, nil
}

// whileParentLives ends ctx when the process that started this one exits: a consumer that
// crashed must not keep bob, nor wait for it. Orphans are adopted by init (or a subreaper), which
// changes the parent; a process started by init itself has no parent to watch.
func whileParentLives(ctx context.Context, interval time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	parent := os.Getppid()
	if parent <= 1 {
		return ctx, cancel
	}
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if os.Getppid() != parent {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}

// runShared prints the shared bob's connection and holds a lease until interrupted, until the
// parent process exits, or until bob goes away.
func runShared(ctx context.Context, config, consumer string, linger time.Duration, out io.Writer) error {
	ctx, cancel := whileParentLives(ctx, time.Second)
	defer cancel()
	directory, err := runtimeDir()
	if err != nil {
		return err
	}
	conn, err := dialBroker(ctx, directory, config, linger)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer conn.Close()
	reply, reader, err := lease(ctx, conn, consumer, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if reply.Config != config {
		fmt.Fprintf(os.Stderr, "bob already runs for this user with %s; using it instead of %s\n", reply.Config, config)
	}
	if err := json.NewEncoder(out).Encode(reply.Connection); err != nil {
		return err
	}

	gone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		close(gone)
	}()
	select {
	case <-ctx.Done():
		return nil
	case <-gone:
		return errors.New("bob stopped: the broker closed the connection")
	}
}
