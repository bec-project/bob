package containermanager

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// hostMCPProcess owns only the foreground process group it started.
type hostMCPProcess struct {
	name     string
	cmd      *exec.Cmd
	done     chan struct{}
	err      error // Written before done closes; read only after receiving from done.
	groupErr error
}

func hostMCPAddress(server MCPServer) (string, error) {
	endpoint, err := getEndpoint(server.URL)
	if err != nil {
		return "", err
	}
	host := endpoint.Hostname()
	switch host {
	case "host.docker.internal", "host.containers.internal":
		host = "127.0.0.1"
	default:
		return "", errors.New("host_command URL must use host.containers.internal (or host.docker.internal), which bob-manager reaches on this machine's loopback")
	}
	port := endpoint.Port()
	if port == "" {
		port = "80"
		if endpoint.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(host, port), nil
}

// hostMCPURL is where this machine reaches an MCP server: the names the sandbox uses for this
// machine become loopback.
func hostMCPURL(server MCPServer) string {
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		return server.URL
	}
	switch endpoint.Hostname() {
	case "host.docker.internal", "host.containers.internal":
		if port := endpoint.Port(); port != "" {
			endpoint.Host = net.JoinHostPort("127.0.0.1", port)
		} else {
			endpoint.Host = "127.0.0.1"
		}
	}
	return endpoint.String()
}

func (m *Manager) startHostMCPs(ctx context.Context, servers map[string]MCPServer) error {
	for _, name := range mcpServerNames(servers) {
		server := servers[name]
		if len(server.HostCommand) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		address, err := hostMCPAddress(server)
		if err != nil {
			return err
		}
		dialer := net.Dialer{Timeout: 200 * time.Millisecond}
		if connection, err := dialer.DialContext(ctx, "tcp", address); err == nil {
			connection.Close()
			return fmt.Errorf("mcp.%s host endpoint is already in use; stop its existing server first", name)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd := exec.Command(server.HostCommand[0], server.HostCommand[1:]...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start host MCP %s: %w", name, err)
		}
		process := &hostMCPProcess{name: name, cmd: cmd, done: make(chan struct{})}
		m.hostMCPs = append(m.hostMCPs, process)
		go func() {
			process.err = cmd.Wait()
			// Reap foreground-command children immediately, not on a later Stop
			// when the exited parent's process-group ID could have been reused.
			process.groupErr = signalHostMCPGroup(cmd.Process.Pid, syscall.SIGKILL)
			close(process.done)
		}()
		ticker := time.NewTicker(100 * time.Millisecond)
		err = func() error {
			defer ticker.Stop()
			for {
				if err := m.hostMCPError(); err != nil {
					return err
				}
				if connection, err := dialer.DialContext(ctx, "tcp", address); err == nil {
					connection.Close()
					return m.hostMCPError()
				}
				select {
				case <-ctx.Done():
					return fmt.Errorf("host MCP %s did not become ready: %w", name, ctx.Err())
				case <-process.done:
					return m.hostMCPError()
				case <-ticker.C:
				}
			}
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) hostMCPError() error {
	for _, process := range m.hostMCPs {
		select {
		case <-process.done:
			if process.err != nil {
				return fmt.Errorf("host MCP %s exited: %w", process.name, process.err)
			}
			return fmt.Errorf("host MCP %s exited before shutdown", process.name)
		default:
		}
	}
	return nil
}

func (m *Manager) stopHostMCPs(parent context.Context) error {
	// SIGKILL can remain pending during uninterruptible host I/O. Keep shutdown
	// bounded and retain ownership if the process has not been reaped in time.
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	var remaining []*hostMCPProcess
	var failures []error
	for _, process := range m.hostMCPs {
		if err := stopHostMCP(ctx, process); err != nil {
			remaining = append(remaining, process)
			failures = append(failures, fmt.Errorf("stop host MCP %s: %w", process.name, err))
		}
	}
	m.hostMCPs = remaining
	return errors.Join(failures...)
}

func stopHostMCP(parent context.Context, process *hostMCPProcess) error {
	select {
	case <-process.done:
		return finishedHostMCPCleanup(parent, process)
	default:
	}
	// Signal the group, so command wrappers and their children cannot survive cleanup.
	if err := signalHostMCPGroup(process.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		return err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-process.done:
		return finishedHostMCPCleanup(parent, process)
	case <-parent.Done():
		return parent.Err()
	case <-timer.C:
	}
	select {
	case <-process.done:
		return finishedHostMCPCleanup(parent, process)
	default:
	}
	if err := signalHostMCPGroup(process.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return err
	}
	select {
	case <-process.done:
		return finishedHostMCPCleanup(parent, process)
	case <-parent.Done():
		return parent.Err()
	}
}

func finishedHostMCPCleanup(parent context.Context, process *hostMCPProcess) error {
	if process.groupErr == nil {
		return nil
	}
	// Once the parent has been reaped, its group ID could have been reused.
	// Retry observation only: never signal that ID again after a cached error.
	exited, err := inspectHostMCPGroup(parent, process.cmd.Process.Pid)
	if err == nil && exited {
		return nil
	}
	return errors.Join(process.groupErr, err)
}

func inspectHostMCPGroup(parent context.Context, pid int) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	// The shared command runner isolates its process group from terminal
	// interrupts and kills that group if the independent inspection times out.
	output, err := command(ctx, "/bin/ps", nil, "-axo", "pgid=,stat=")
	if err != nil {
		return false, err
	}
	return hostMCPGroupExited(output, pid), nil
}

func signalHostMCPGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if runtime.GOOS == "darwin" && errors.Is(err, syscall.EPERM) {
		// Darwin excludes zombies when signalling a group, returning EPERM
		// when no live members remain. Do not swallow genuine permission errors.
		exited, inspectErr := inspectHostMCPGroup(context.Background(), pid)
		if inspectErr == nil && exited {
			return nil
		}
	}
	return err
}

func hostMCPGroupExited(output string, pid int) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return false
		}
		group, err := strconv.Atoi(fields[0])
		if err != nil || fields[1] == "" {
			return false
		}
		if group == pid && fields[1][0] != 'Z' {
			return false
		}
	}
	return true
}
