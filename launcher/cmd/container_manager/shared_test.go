package main

import (
	"bufio"
	"context"
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

// fakeLifecycle stands in for the Manager: it counts starts and stops and can block or fail.
type fakeLifecycle struct {
	mu      sync.Mutex
	starts  int
	stops   int
	block   bool
	failure error
	running bool
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

func TestSharedBobStartsOnceAndStopsAfterTheLastConsumer(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 100*time.Millisecond)
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

func TestConsumerDuringLingerKeepsBob(t *testing.T) {
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

func TestStartFailureReachesEveryConsumer(t *testing.T) {
	manager := &fakeLifecycle{failure: errors.New("start the docker engine first")}
	run := startBroker(t, manager, time.Second)
	_, _, err := consume(t, run.socket, "gui")
	if err == nil || !strings.Contains(err.Error(), "docker engine") {
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
	// the waiting consumer learns that bob is gone: its connection closes, or, when the
	// broker read its hello only after stopping began, it gets an error reply
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err == nil && !strings.Contains(string(line), `"error"`) {
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

func TestSharedFlags(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "bob.toml")
	content := "[workspace]\nworkdir = \".\"\n[provider]\nbase_url = \"http://127.0.0.1:12345/api/\"\n"
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	call, err := parseArgs([]string{"--config", config, "--shared", "--linger", "30s", "--consumer", "gui"})
	if err != nil {
		t.Fatal(err)
	}
	if !call.shared || call.broker || call.linger != 30*time.Second || call.consumer != "gui" || call.config != config {
		t.Fatalf("unexpected invocation: %+v", call)
	}
	if call, err := parseArgs([]string{"--config", config}); err != nil || call.shared || call.linger != defaultLinger {
		t.Fatalf("classic mode changed: %+v %v", call, err)
	}
	if _, err := parseArgs([]string{"--config", config, "--shared", "--linger", "-1s"}); err == nil {
		t.Fatal("accepted a negative linger")
	}
}

// lockedBuffer collects a consumer's output while the test reads it.
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

// startConsumer runs runShared against the broker's runtime directory, as `--shared` does.
func startConsumer(t *testing.T, ctx context.Context, run brokerRun, config string) (*lockedBuffer, chan error) {
	t.Helper()
	t.Setenv("BOB_RUNTIME_DIR", filepath.Dir(run.socket))
	out := &lockedBuffer{}
	done := make(chan error, 1)
	go func() { done <- runShared(ctx, config, "test", time.Second, out) }()
	return out, done
}

func TestConsumerPrintsTheConnectionAndHoldsItsLeaseUntilInterrupted(t *testing.T) {
	manager := &fakeLifecycle{}
	run := startBroker(t, manager, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, done := startConsumer(t, ctx, run, "/etc/bob.toml")
	waitFor(t, "the connection line", func() bool {
		return strings.Contains(out.String(), `"url":"http://127.0.0.1:4100"`)
	})
	if _, stops := manager.counts(); stops != 0 {
		t.Fatal("bob stopped while the consumer held it")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("interrupted consumer returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer ignored the interrupt")
	}
	waitFor(t, "bob to stop after the lease ended", func() bool {
		_, stops := manager.counts()
		return stops == 1
	})
}

func TestConsumerFailsWhenTheBrokerGoesAway(t *testing.T) {
	run := startBroker(t, &fakeLifecycle{}, time.Minute)
	// A different bob.toml joins the running bob with a warning instead of a second bob.
	out, done := startConsumer(t, context.Background(), run, "/other/bob.toml")
	waitFor(t, "the connection line", func() bool { return strings.Contains(out.String(), `"url"`) })
	run.cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "the broker closed the connection") {
			t.Fatalf("consumer outlived its broker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer kept waiting for a broker that stopped")
	}
}

func TestConsumerReportsAStartFailure(t *testing.T) {
	run := startBroker(t, &fakeLifecycle{failure: errors.New("no engine")}, time.Minute)
	_, done := startConsumer(t, context.Background(), run, "/etc/bob.toml")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "no engine") {
			t.Fatalf("start failure did not reach the consumer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer waited after bob failed to start")
	}
}

func TestConsumerStartsABrokerWhenNoneAnswers(t *testing.T) {
	directory, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("BOB_RUNTIME_DIR", directory)
	// The spawned broker is this test binary, which rejects --broker and exits; the consumer
	// keeps waiting until it is interrupted, which is not an error.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := runShared(ctx, "/etc/bob.toml", "test", time.Second, io.Discard); err != nil {
		t.Fatalf("interrupted consumer returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, brokerLog)); err != nil {
		t.Fatalf("no broker was started: %v", err)
	}
}

func TestConsumerNoticesItsParentExit(t *testing.T) {
	const name = "^TestConsumerNoticesItsParentExit$"
	switch os.Getenv("BOB_PARENT_HELPER") {
	case "parent":
		// Start the consumer, wait until it watches this process, then exit under it.
		child := exec.Command(os.Args[0], "-test.run="+name)
		child.Env = append(os.Environ(), "BOB_PARENT_HELPER=child")
		child.Stdout = os.Stdout
		ready, err := child.StderrPipe()
		if err == nil {
			err = child.Start()
		}
		if err == nil {
			_, err = bufio.NewReader(ready).ReadString('\n')
		}
		if err != nil {
			fmt.Println("helper failed:", err)
		}
		os.Exit(0)
	case "child":
		ctx, cancel := whileParentLives(context.Background(), 20*time.Millisecond)
		defer cancel()
		fmt.Fprintln(os.Stderr, "watching")
		select {
		case <-ctx.Done():
			fmt.Println("parent gone")
		case <-time.After(10 * time.Second):
			fmt.Println("parent exit missed")
		}
		os.Exit(0)
	}
	parent := exec.Command(os.Args[0], "-test.run="+name)
	parent.Env = append(os.Environ(), "BOB_PARENT_HELPER=parent")
	stdout, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	// The pipe closes once the consumer exits too, so this reads its verdict.
	output, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Wait(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "parent gone") {
		t.Fatalf("consumer did not notice its parent exit: %q", output)
	}
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
