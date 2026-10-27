package main

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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"bob/container_manager"
)

// fakeLifecycle stands in for the Manager: it counts starts, checks and stops, and can block or
// fail; checks return checkErr.
type fakeLifecycle struct {
	mu       sync.Mutex
	starts   int
	checks   int
	stops    int
	block    bool
	failure  error
	checkErr error
	running  bool
}

func (f *fakeLifecycle) Check(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stops > 0 {
		panic("Check after Stop")
	}
	f.checks++
	return f.checkErr
}

func (f *fakeLifecycle) setCheck(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkErr = err
}

func (f *fakeLifecycle) checkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks
}

func (f *fakeLifecycle) Start(ctx context.Context, _ containermanager.Options) (containermanager.Connection, error) {
	f.mu.Lock()
	f.starts++
	block, failure := f.block, f.failure
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return containermanager.Connection{}, ctx.Err()
	}
	if failure != nil {
		return containermanager.Connection{}, failure
	}
	f.mu.Lock()
	f.running = true
	f.mu.Unlock()
	return containermanager.Connection{URL: "http://127.0.0.1:4100", Password: "pw", Container: "bob-test"}, nil
}

func (f *fakeLifecycle) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	f.running = false
	return nil
}

func (f *fakeLifecycle) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.stops
}

type brokerRun struct {
	socket string
	done   chan error
	cancel context.CancelFunc
}

func startBroker(t *testing.T, manager lifecycle, linger time.Duration) brokerRun {
	t.Helper()
	// Unix socket paths are short; t.TempDir can exceed the limit on macOS.
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, brokerSocket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := brokerRun{socket: socket, done: make(chan error, 1), cancel: cancel}
	b := newBroker(manager, containermanager.Options{}, "/etc/bob.toml", linger, io.Discard)
	go func() {
		run.done <- b.serve(ctx, listener)
		close(run.done) // later receives (the cleanup below) return at once
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-run.done:
		case <-time.After(5 * time.Second):
			t.Error("broker did not stop")
		}
	})
	return run
}

// lease is a client's side of the protocol, as the GUIs implement it: send hello, read the reply.
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

func consume(t *testing.T, socket, name string) (brokerReply, net.Conn, error) {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reply, _, err := lease(ctx, conn, name, "/etc/bob.toml")
	if err != nil {
		conn.Close()
		return reply, nil, err
	}
	return reply, conn, nil
}

func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBobStartsWithTheFirstClientAndStopsAfterTheLast(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 100*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	if starts, _ := manager.counts(); starts != 0 {
		t.Fatal("the broker started bob before any client asked for it")
	}
	first, firstConn, err := consume(t, run.socket, "gui")
	if err != nil {
		t.Fatal(err)
	}
	second, secondConn, err := consume(t, run.socket, "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if first.Connection.Container != "bob-test" || second.Connection.Password != "pw" || second.Config != "/etc/bob.toml" {
		t.Fatalf("unexpected replies: %+v %+v", first, second)
	}
	if starts, _ := manager.counts(); starts != 1 {
		t.Fatalf("bob started %d times for two consumers", starts)
	}
	firstConn.Close()
	time.Sleep(300 * time.Millisecond)
	if _, stops := manager.counts(); stops != 0 {
		t.Fatal("bob stopped while a consumer still held it")
	}
	secondConn.Close()
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bob outlived its last consumer")
	}
	if starts, stops := manager.counts(); starts != 1 || stops != 1 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
}

func TestClientDuringLingerKeepsBob(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 400*time.Millisecond)
	_, conn, err := consume(t, run.socket, "gui")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	time.Sleep(100 * time.Millisecond)
	_, again, err := consume(t, run.socket, "gui-restarted")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	if starts, stops := manager.counts(); starts != 1 || stops != 0 {
		t.Fatalf("a consumer returning within the linger time lost bob: starts=%d stops=%d", starts, stops)
	}
	again.Close()
	waitFor(t, "bob to stop", func() bool { _, stops := manager.counts(); return stops == 1 })
}

// fastHealthChecks makes the broker check bob every interval during a test.
func fastHealthChecks(t *testing.T, interval time.Duration) {
	t.Helper()
	previous := healthInterval
	healthInterval = interval
	t.Cleanup(func() { healthInterval = previous })
}

func TestBrokerEndsEveryLeaseWhenBobStops(t *testing.T) {
	fastHealthChecks(t, 10*time.Millisecond)
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, time.Minute)
	_, first, err := consume(t, run.socket, "gui")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, second, err := consume(t, run.socket, "widgets")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	waitFor(t, "health checks", func() bool { return manager.checkCount() >= 3 })
	manager.setCheck(fmt.Errorf("%w: its container is gone", containermanager.ErrStopped))
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the broker kept serving a bob whose container is gone")
	}
	for _, conn := range []net.Conn{first, second} {
		if reason := stoppedThenClosed(t, conn); reason != "bob stopped: its container is gone" {
			t.Fatalf("a client was told %q", reason)
		}
	}
	if starts, stops := manager.counts(); starts != 1 || stops != 1 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
}

// stoppedThenClosed reads what a lease gets when bob stops: why, then the end of the connection.
func stoppedThenClosed(t *testing.T, conn net.Conn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("the lease ended without saying why: %v", err)
	}
	var reply brokerReply
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatalf("unexpected line %q: %v", line, err)
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("a lease outlived bob: %v", err)
	}
	return reply.Stopped
}

func TestClientStopsBobForEveryone(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, time.Minute)
	_, first, err := consume(t, run.socket, "gui")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, second, err := consume(t, run.socket, "widgets")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	conn, err := net.Dial("unix", run.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	request := hello{Consumer: "bob-gui", PID: os.Getpid(), Stop: true, Reason: "restarted with /etc/other.toml"}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		t.Fatal(err)
	}
	if reason := stoppedThenClosed(t, conn); reason != "restarted with /etc/other.toml" {
		t.Fatalf("the stopping client was told %q", reason)
	}
	for _, lease := range []net.Conn{first, second} {
		if reason := stoppedThenClosed(t, lease); reason != "restarted with /etc/other.toml" {
			t.Fatalf("a client was told %q", reason)
		}
	}
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the broker kept running after a client stopped bob")
	}
	if starts, stops := manager.counts(); starts != 1 || stops != 1 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
}

func TestStopWithoutBobJustEndsTheBroker(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, time.Minute)
	conn, err := net.Dial("unix", run.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(hello{Consumer: "bob-gui", Stop: true}); err != nil {
		t.Fatal(err)
	}
	if reason := stoppedThenClosed(t, conn); reason != "bob was stopped by bob-gui" {
		t.Fatalf("the stopping client was told %q", reason)
	}
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the broker kept running")
	}
	if starts, stops := manager.counts(); starts != 0 || stops != 0 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
}

func TestBrokerOutlastsBriefEngineFailures(t *testing.T) {
	// slow enough that a good check comes back long before the limit of failures in a row
	fastHealthChecks(t, 25*time.Millisecond)
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, time.Minute)
	_, conn, err := consume(t, run.socket, "gui")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// failures add up to more than the limit, but never that many in a row: bob stays
	for round := 0; round < 3; round++ {
		manager.setCheck(errors.New("the sandbox could not be checked"))
		checked := manager.checkCount()
		waitFor(t, "failing checks", func() bool { return manager.checkCount() >= checked+2 })
		manager.setCheck(nil)
		checked = manager.checkCount()
		waitFor(t, "a good check", func() bool { return manager.checkCount() >= checked+2 })
	}
	select {
	case <-run.done:
		t.Fatal("the broker stopped bob over a few failed checks")
	default:
	}
	// an engine that stays away counts as bob gone
	manager.setCheck(errors.New("the sandbox could not be checked"))
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the broker kept serving while the engine did not answer")
	}
	if _, stops := manager.counts(); stops != 1 {
		t.Fatal("no cleanup after the engine went away")
	}
}

func TestStartFailureReachesEveryClient(t *testing.T) {
	manager := &fakeLifecycle{failure: errors.New("find bwrap on PATH; install bubblewrap")}
	run := startBroker(t, manager, time.Second)
	_, _, err := consume(t, run.socket, "gui")
	if err == nil || !strings.Contains(err.Error(), "install bubblewrap") {
		t.Fatalf("consumer did not get the start error: %v", err)
	}
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		t.Fatal("broker kept running after bob failed to start")
	}
}

func TestInterruptCancelsStartupBeforeStopping(t *testing.T) {
	manager := &fakeLifecycle{block: true}
	run := startBroker(t, manager, time.Second)
	conn, err := net.Dial("unix", run.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(`{"consumer":"gui","pid":1,"config":"/etc/bob.toml"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "startup to begin", func() bool { starts, _ := manager.counts(); return starts == 1 })
	run.cancel()
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not cancel the startup")
	}
	if _, stops := manager.counts(); stops != 1 {
		t.Fatal("cleanup did not run after the cancelled startup")
	}
	// the waiting consumer learns that bob is gone: its connection closes, it is told why bob
	// stopped, or, when the broker read its hello only after stopping began, it gets an error reply
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err == nil && !strings.Contains(string(line), `"error"`) && !strings.Contains(string(line), `"stopped"`) {
		t.Fatalf("consumer got a connection from a stopping broker: %s", line)
	}
}

func TestSecondBrokerLeavesTheLockHolderAlone(t *testing.T) {
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOB_RUNTIME_DIR", directory)
	lock, err := os.OpenFile(filepath.Join(directory, brokerLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runBroker(ctx, "/etc/bob.toml", containermanager.Options{}, time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, brokerSocket)); !os.IsNotExist(err) {
		t.Fatal("a second broker opened a socket")
	}
}

func TestRuntimeDirMustBePrivate(t *testing.T) {
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("BOB_RUNTIME_DIR", directory)
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeDir(); err == nil {
		t.Fatal("accepted a directory other users can read")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := runtimeDir(); err != nil || got != directory {
		t.Fatalf("runtimeDir() = %q, %v", got, err)
	}
}

func TestRuntimeDirRejectsSocketPathsTooLong(t *testing.T) {
	directory := filepath.Join(t.TempDir(), strings.Repeat("d", maxSocketPath()))
	t.Setenv("BOB_RUNTIME_DIR", directory)
	if _, err := runtimeDir(); err == nil || !strings.Contains(err.Error(), "too long for a Unix socket") {
		t.Fatalf("runtimeDir() accepted a socket path the broker cannot listen on: %v", err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("created the unusable directory: %v", err)
	}
}

func TestBrokerFlags(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "bob.toml")
	content := "[workspace]\nworkdir = \".\"\n[provider]\nbase_url = \"http://127.0.0.1:12345/api/\"\n"
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	call, err := parseArgs([]string{"--config", config, "--broker", "--linger", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	if !call.broker || call.linger != 30*time.Second || call.config != config {
		t.Fatalf("unexpected invocation: %+v", call)
	}
	if call, err := parseArgs([]string{"--config", config}); err != nil || call.broker || call.linger != defaultLinger {
		t.Fatalf("default invocation changed: %+v %v", call, err)
	}
	if _, err := parseArgs([]string{"--config", config, "--linger", "-1s"}); err == nil {
		t.Fatal("accepted a negative linger")
	}
	// bob has no headless mode any more: the client flags are gone
	for _, flag := range []string{"--shared", "--consumer=cli"} {
		if _, err := parseArgs([]string{"--config", config, flag}); err == nil {
			t.Errorf("accepted the removed %s flag", flag)
		}
	}
}

// lockedBuffer collects output while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	out strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.out.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.out.String()
}

func TestLocatePrintsTheSocketOfTheRunningBroker(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, time.Minute)
	t.Setenv("BOB_RUNTIME_DIR", filepath.Dir(run.socket))
	out := &lockedBuffer{}
	if err := runLocate(context.Background(), "/etc/bob.toml", time.Second, out); err != nil {
		t.Fatal(err)
	}
	var located struct {
		Socket string `json:"socket"`
	}
	if err := json.Unmarshal([]byte(out.String()), &located); err != nil || located.Socket != run.socket {
		t.Fatalf("located %q (%v), want %s", out.String(), err, run.socket)
	}
	// Checking that the broker answers holds no lease and starts nothing.
	time.Sleep(50 * time.Millisecond)
	if starts, _ := manager.counts(); starts != 0 {
		t.Fatal("locating the broker started bob")
	}
}

func TestLocateStartsABrokerWhenNoneAnswers(t *testing.T) {
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("BOB_RUNTIME_DIR", directory)
	// The spawned broker is this test binary, which rejects --broker and exits; locating keeps
	// waiting until it is interrupted, which is not an error.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := runLocate(ctx, "/etc/bob.toml", time.Second, io.Discard); err != nil {
		t.Fatalf("interrupted locate returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, brokerLog)); err != nil {
		t.Fatalf("no broker was started: %v", err)
	}
}

func TestBrokerWithoutClientsStopsWithoutStartingBob(t *testing.T) {
	previous := firstClientWait
	firstClientWait = 50 * time.Millisecond
	t.Cleanup(func() { firstClientWait = previous })
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 10*time.Millisecond)
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a broker nobody used kept running")
	}
	if starts, stops := manager.counts(); starts != 0 || stops != 0 {
		t.Fatalf("a broker nobody used touched bob: starts=%d stops=%d", starts, stops)
	}
}

func TestClientThatDiesEndsItsLease(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 50*time.Millisecond)
	// A client in another process holds the lease through its open connection only.
	client := exec.Command(os.Args[0], "-test.run=^TestClientHelper$")
	client.Env = append(os.Environ(), "BOB_CLIENT_HELPER="+run.socket)
	stdout, err := client.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || !strings.Contains(line, "leased") {
		t.Fatalf("helper client did not get bob: %q %v", line, err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, stops := manager.counts(); stops != 0 {
		t.Fatal("bob stopped while the client held it")
	}
	_ = client.Process.Kill()
	_ = client.Wait()
	waitFor(t, "bob to stop after the killed client's lease", func() bool {
		_, stops := manager.counts()
		return stops == 1
	})
}

func TestClientHelper(t *testing.T) {
	socket := os.Getenv("BOB_CLIENT_HELPER")
	if socket == "" {
		return
	}
	conn, err := net.Dial("unix", socket)
	if err == nil {
		_, _, err = lease(context.Background(), conn, "helper", "/etc/bob.toml")
	}
	if err != nil {
		fmt.Println("failed:", err)
		os.Exit(1)
	}
	fmt.Println("leased")
	time.Sleep(time.Minute) // killed by the test
	os.Exit(0)
}

func TestBrokerServesOnItsSocketAndRemovesIt(t *testing.T) {
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("BOB_RUNTIME_DIR", directory)
	socket := filepath.Join(directory, brokerSocket)
	// A socket left by a broker that died is replaced: the lock proves nobody uses it.
	if err := os.WriteFile(socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	// Without a provider URL the real Manager fails before it touches an engine.
	go func() {
		done <- runBroker(context.Background(), "/etc/bob.toml", containermanager.Options{}, time.Second)
	}()
	var reply brokerReply
	waitFor(t, "the broker to answer", func() bool {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return false
		}
		defer conn.Close()
		reply, _, err = lease(context.Background(), conn, "test", "/etc/bob.toml")
		return err != nil
	})
	if reply.Error == "" {
		t.Fatal("the start failure did not reach the consumer")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the broker kept running after bob failed to start")
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("the broker left its socket: %v", err)
	}
}

func TestRuntimeDirFallsBackToTheUsersTemporaryDirectories(t *testing.T) {
	base, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	t.Setenv("BOB_RUNTIME_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", base)
	if got, err := runtimeDir(); err != nil || got != filepath.Join(base, "bob") {
		t.Fatalf("with XDG_RUNTIME_DIR: %q, %v", got, err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", base)
	want := filepath.Join(base, fmt.Sprintf("bob-%d", os.Getuid()))
	if runtime.GOOS == "darwin" {
		want = filepath.Join(base, "bob")
	}
	got, err := runtimeDir()
	if err != nil || got != want {
		t.Fatalf("without XDG_RUNTIME_DIR: %q, %v; want %q", got, err, want)
	}
	if info, err := os.Stat(got); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("runtime directory is not private: %v, %v", info, err)
	}
}
