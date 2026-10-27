package containermanager

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func hostMCPFixture(t *testing.T) (MCPServer, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	server := MCPServer{
		URL: "http://host.docker.internal:" + strconv.Itoa(port) + "/mcp",
		HostCommand: []string{python, "-u", "-c", `import socketserver,sys
socketserver.TCPServer(("127.0.0.1",int(sys.argv[1])),socketserver.BaseRequestHandler).serve_forever()
`, strconv.Itoa(port)},
	}
	return server, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func assertHostPortClosed(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("host MCP listener survived cleanup")
	}
}

func TestHostMCPGroupExitedRequiresNoLiveMembers(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		exited bool
	}{
		{"absent", "99 S\n", true},
		{"zombie", "42 Z\n42 Z+\n99 S\n", true},
		{"live", "42 S\n", false},
		{"mixed", "42 Z\n42 R\n", false},
		{"unknown", "42 ?\n", false},
		{"malformed", "unexpected output\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hostMCPGroupExited(test.output, 42); got != test.exited {
				t.Fatalf("group exited = %v, want %v", got, test.exited)
			}
		})
	}
}

func TestHostMCPCleanupRetriesFailedInspection(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	close(done)
	manager := &Manager{hostMCPs: []*hostMCPProcess{{
		name: "bec_mcp", cmd: cmd, done: done, groupErr: syscall.EPERM,
	}}}
	// Model an inspection failure after the process was reaped, then another
	// failed inspection during cleanup. Neither should settle ownership.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Stop(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost inspection cancellation: %v", err)
	}
	if len(manager.hostMCPs) != 1 {
		t.Fatal("failed inspection discarded process ownership")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("cleanup retry retained stale inspection error: %v", err)
	}
	if len(manager.hostMCPs) != 0 {
		t.Fatal("successful inspection retained process ownership")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal("completed cleanup was not idempotent")
	}
}

func TestHostMCPCleanupRetryDoesNotSignalLiveGroup(t *testing.T) {
	server, address := hostMCPFixture(t)
	owner := &Manager{}
	t.Cleanup(func() { _ = owner.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := owner.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server}); err != nil {
		t.Fatal(err)
	}
	// A completed process with a failed group cleanup may now refer to a live
	// reused group. Borrow this test-owned server's ID to verify observation only.
	done := make(chan struct{})
	close(done)
	manager := &Manager{hostMCPs: []*hostMCPProcess{{
		name: "completed", cmd: owner.hostMCPs[0].cmd, done: done, groupErr: syscall.EPERM,
	}}}
	if err := manager.Stop(context.Background()); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("live group was incorrectly settled: %v", err)
	}
	if len(manager.hostMCPs) != 1 {
		t.Fatal("unresolved group discarded ownership")
	}
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal("cleanup retry signalled the live process group")
	}
	connection.Close()
}

func TestHostMCPFixtureDoesNotResolveHostnames(t *testing.T) {
	server, address := hostMCPFixture(t)
	server.HostCommand[3] = "import socket\ndef reject_dns(*args):\n raise RuntimeError('fixture must not resolve hostnames')\nsocket.getfqdn = reject_dns\n" + server.HostCommand[3]
	manager := &Manager{}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertHostPortClosed(t, address)
}

func TestHostMCPDarwinZombieGroupCleanup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin reports EPERM for zombie-only process groups")
	}
	server, _ := hostMCPFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(ctx, server.HostCommand[0], "-u", "-c", `import os,sys
pid = os.fork()
if pid == 0:
    os.setpgid(0, 0)
    os._exit(0)
os.waitid(os.P_PID, pid, os.WEXITED | os.WNOWAIT)
print(pid, flush=True)
sys.stdin.readline()
os.waitpid(pid, 0)
`)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Release the synthetic parent so it reaps its own zombie child.
		_ = stdin.Close()
		_ = cmd.Wait()
		cancel()
	})
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("zombie fixture did not report its process group")
	}
	pid, err := strconv.Atoi(scanner.Text())
	if err != nil || pid <= 1 {
		t.Fatalf("invalid fixture process group: %q", scanner.Text())
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.EPERM) {
		t.Fatalf("unexpected zombie-only group signal result: %v", err)
	}
	if err := signalHostMCPGroup(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("zombie-only group cleanup failed: %v", err)
	}
}

func TestHostMCPStartsAndStopsWithManager(t *testing.T) {
	server, address := hostMCPFixture(t)
	manager := &Manager{}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server}); err != nil {
		t.Fatal(err)
	}
	if len(manager.hostMCPs) != 1 {
		t.Fatal("host process ownership was not recorded")
	}
	if _, err := manager.Start(ctx, Options{}); err == nil {
		t.Fatal("allowed a second start while a host MCP is owned")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal("stop was not idempotent")
	}
	assertHostPortClosed(t, address)
}

func TestHostMCPOccupiedEndpointDoesNotKillExistingService(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server, _ := hostMCPFixture(t)
	server.URL = "http://host.docker.internal:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) + "/mcp"
	manager := &Manager{}
	err = manager.startHostMCPs(context.Background(), map[string]MCPServer{"bec_mcp": server})
	if err == nil || !strings.Contains(err.Error(), "already in use") || len(manager.hostMCPs) != 0 {
		t.Fatal("existing service was adopted or replaced")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal("cleanup touched unrelated service")
	}
	connection.Close()
}

func TestHostMCPEarlyExitAndCancelledStartupCleanUp(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelled), func(t *testing.T) {
			server, address := hostMCPFixture(t)
			server.HostCommand = []string{"sh", "-c", "exit 7"}
			ctx := context.Background()
			if cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 150*time.Millisecond)
				defer cancel()
				server.HostCommand = []string{"sh", "-c", "sleep 30 & wait"}
			}
			manager := &Manager{}
			err := manager.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server})
			if cancelled && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if !cancelled && (err == nil || !strings.Contains(err.Error(), "exit status 7")) {
				t.Fatalf("lost host exit status: %v", err)
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(manager.hostMCPs) != 0 {
				t.Fatal("cleanup retained process ownership")
			}
			assertHostPortClosed(t, address)
		})
	}
}

func TestHostMCPStartupFailureRemovesOwnedProcess(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, address := hostMCPFixture(t)
	options.MCPServers = map[string]MCPServer{"bec_mcp": server}
	manager := &Manager{}
	_, err := manager.Start(context.Background(), options)
	if err == nil || !strings.Contains(err.Error(), "exited before becoming ready") {
		t.Fatalf("expected the sandbox to fail: %v", err)
	}
	assertHostPortClosed(t, address)
	if len(manager.hostMCPs) != 0 {
		t.Fatal("startup failure retained host MCP ownership")
	}
}

func TestHostMCPInheritedDirectoryCannotBeAgentWritable(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, _ := hostMCPFixture(t)
	for _, directory := range []string{options.Workdir, filepath.Join(t.TempDir(), "alias")} {
		if directory != options.Workdir {
			if err := os.Symlink(options.Workdir, directory); err != nil {
				t.Fatal(err)
			}
		}
		t.Chdir(directory)
		options.MCPServers = map[string]MCPServer{"bec_mcp": server}
		_, err := getResolvedOptions(options)
		if err == nil || !strings.Contains(err.Error(), "must not overlap") {
			t.Fatalf("allowed host code in agent-writable directory: %v", err)
		}
	}
}

func TestHostMCPCleanupCanBeRetriedAfterCancellation(t *testing.T) {
	server, address := hostMCPFixture(t)
	server.HostCommand[3] = `import socketserver,signal,sys
signal.signal(signal.SIGTERM,signal.SIG_IGN)
socketserver.TCPServer(("127.0.0.1",int(sys.argv[1])),socketserver.BaseRequestHandler).serve_forever()
`
	manager := &Manager{}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server}); err != nil {
		t.Fatal(err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := manager.Stop(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup cancellation was not retained: %v", err)
	}
	if len(manager.hostMCPs) != 1 {
		t.Fatal("unresolved cleanup discarded host ownership")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertHostPortClosed(t, address)
}

func TestHostMCPExecutableCannotBeAgentWritable(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, _ := hostMCPFixture(t)
	executable := filepath.Join(options.Workdir, "host-mcp")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "host-mcp-alias")
	if err := os.Symlink(executable, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", options.Workdir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, command := range []string{executable, alias, "host-mcp"} {
		server.HostCommand = []string{command}
		options.MCPServers = map[string]MCPServer{"bec_mcp": server}
		_, err := getResolvedOptions(options)
		if err == nil || !strings.Contains(err.Error(), "host executable must not overlap") {
			t.Fatalf("allowed agent-writable host executable %q: %v", command, err)
		}
	}
}

func TestHostMCPKeepsValidatedExecutableAfterPATHShadowAppears(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, _ := hostMCPFixture(t)
	installedDirectory := t.TempDir()
	installed := filepath.Join(installedDirectory, "host-mcp")
	if err := os.WriteFile(installed, []byte("#!/bin/sh\necho safe\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{options.Workdir, installedDirectory, os.Getenv("PATH")}, string(os.PathListSeparator)))
	server.HostCommand = []string{"host-mcp"}
	options.MCPServers = map[string]MCPServer{"bec_mcp": server}
	resolved, err := getResolvedOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.Workdir, "host-mcp"), []byte("#!/bin/sh\necho unsafe\n"), 0700); err != nil {
		t.Fatal(err)
	}
	command := resolved.MCPServers["bec_mcp"].HostCommand
	output, err := exec.Command(command[0], command[1:]...).Output()
	if err != nil || strings.TrimSpace(string(output)) != "safe" {
		t.Fatalf("executed PATH shadow after validation: output=%q error=%v", output, err)
	}
	if options.MCPServers["bec_mcp"].HostCommand[0] != "host-mcp" {
		t.Fatal("resolution mutated caller's command")
	}
}

func TestHostMCPRejectsWritableIntermediateSymlink(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, _ := hostMCPFixture(t)
	installed := filepath.Join(t.TempDir(), "installed-mcp")
	if err := os.WriteFile(installed, []byte("#!/bin/sh\necho safe\n"), 0700); err != nil {
		t.Fatal(err)
	}
	intermediate := filepath.Join(options.Workdir, "intermediate-mcp")
	if err := os.Symlink(installed, intermediate); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "host-mcp")
	if err := os.Symlink(intermediate, alias); err != nil {
		t.Fatal(err)
	}
	server.HostCommand = []string{alias}
	options.MCPServers = map[string]MCPServer{"bec_mcp": server}
	_, err := getResolvedOptions(options)
	if err == nil || !strings.Contains(err.Error(), "intermediate symlinks") {
		t.Fatalf("accepted mutable intermediate symlink: %v", err)
	}
}

func TestHostMCPPreservesVirtualenvInterpreter(t *testing.T) {
	options, _ := sandboxEngine(t)
	server, _ := hostMCPFixture(t)
	venv := filepath.Join(t.TempDir(), "venv")
	if output, err := exec.Command(server.HostCommand[0], "-m", "venv", "--without-pip", "--symlinks", venv).CombinedOutput(); err != nil {
		t.Fatalf("create virtualenv: %v: %s", err, output)
	}
	server.HostCommand = []string{filepath.Join(venv, "bin", "python"), "-c", "import sys; print(sys.prefix)"}
	options.MCPServers = map[string]MCPServer{"bec_mcp": server}
	resolved, err := getResolvedOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	command := resolved.MCPServers["bec_mcp"].HostCommand
	output, err := exec.Command(command[0], command[1:]...).Output()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(venv)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("host interpreter lost virtualenv: got %q want %q", actual, expected)
	}
}

func TestHostMCPInheritsLauncherDirectory(t *testing.T) {
	server, _ := hostMCPFixture(t)
	directory := t.TempDir()
	marker := filepath.Join(directory, "host-cwd")
	t.Chdir(directory)
	server.HostCommand[3] = `import socketserver,os,pathlib,sys
pathlib.Path(sys.argv[2]).write_text(os.getcwd())
socketserver.TCPServer(("127.0.0.1",int(sys.argv[1])),socketserver.BaseRequestHandler).serve_forever()
`
	server.HostCommand = append(server.HostCommand, marker)
	manager := &Manager{}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.startHostMCPs(ctx, map[string]MCPServer{"bec_mcp": server}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != expected {
		t.Fatalf("host MCP directory %q differs from launch directory %q", got, expected)
	}
}
