package containermanager

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The bubblewrap runtime runs OpenCode on the host, in a sandbox of its own user, mount, PID,
// IPC, UTS and network namespaces. It sees /usr and /etc read-only, the workspace mounts and
// its own home, nothing else of the host, and has no network: tunnel.go carries the few
// connections it needs. It needs no image, no engine and no kernel modules.

const bubblewrapRuntime = "bubblewrap"

const (
	// sandboxHome is OpenCode's home in the sandbox.
	sandboxHome = "/home/opencode"
	// sandboxBin holds the executables the sandbox runs: bob-manager, as the helper, and
	// OpenCode.
	sandboxBin = "/bob/bin"
	// sandboxServer is OpenCode's address in the sandbox.
	sandboxServer = "127.0.0.1:4096"
)

// openCodeConfig is containers/opencode.json, the configuration the image carries.
//
//go:embed opencode.json
var openCodeConfig []byte

// sandboxProcess is a running sandbox and the tunnels bob-manager serves for it.
type sandboxProcess struct {
	cmd       *exec.Cmd
	done      chan struct{}
	err       error // Written before done closes; read only after receiving from done.
	output    *tailWriter
	runDir    string
	listeners []net.Listener
}

// startBubblewrap starts OpenCode in a sandbox and waits until it answers.
func (m *Manager) startBubblewrap(parent context.Context, options resolvedOptions) (connection Connection, startErr error) {
	if runtime.GOOS != "linux" {
		return connection, errors.New("the bubblewrap runtime runs on Linux only; use Docker or Podman here")
	}
	for _, name := range mcpServerNames(options.MCPServers) {
		if options.MCPServers[name].Port != 0 {
			return connection, fmt.Errorf("mcp.%s.port publishes a container port, which the bubblewrap runtime has none of", name)
		}
	}
	openCode, err := exec.LookPath(options.OpenCode)
	if err != nil {
		return connection, fmt.Errorf("find OpenCode (container.opencode): %w", err)
	}
	if openCode, err = filepath.Abs(openCode); err != nil {
		return connection, err
	}
	helper, err := os.Executable()
	if err != nil {
		return connection, err
	}
	if err := os.MkdirAll(options.Home, 0o700); err != nil {
		return connection, fmt.Errorf("create bob's home folder: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	defer func() {
		if startErr != nil {
			if err := m.Stop(context.Background()); err != nil {
				startErr = errors.Join(startErr, fmt.Errorf("startup cleanup: %w", err))
			}
		}
	}()
	if err := m.startHostMCPs(ctx, options.MCPServers); err != nil {
		return connection, err
	}

	urls := []string{options.BaseURL}
	for _, name := range mcpServerNames(options.MCPServers) {
		if server := options.MCPServers[name]; server.URL != "" {
			urls = append(urls, server.URL)
		}
	}
	endpoints, inside, hosts, err := getSandboxEndpoints(urls)
	if err != nil {
		return connection, err
	}
	runDir, err := os.MkdirTemp(sandboxRunBase(), "bob-")
	if err != nil {
		return connection, err
	}
	sandbox := &sandboxProcess{runDir: runDir, done: make(chan struct{}), output: &tailWriter{}}
	m.sandbox = sandbox
	hostsFile := "127.0.0.1 localhost\n::1 localhost\n" + strings.Join(hosts, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(runDir, "hosts"), []byte(hostsFile), 0o644); err != nil {
		return connection, err
	}
	if err := os.WriteFile(filepath.Join(runDir, "opencode.json"), openCodeConfig, 0o644); err != nil {
		return connection, err
	}
	for _, endpoint := range endpoints {
		listener, err := net.Listen("unix", filepath.Join(runDir, endpoint.Socket))
		if err != nil {
			return connection, err
		}
		sandbox.listeners = append(sandbox.listeners, listener)
		go serveTunnel(listener, dialTCP(endpoint.target))
	}

	env, err := getSandboxEnvironment(options, inside, endpoints)
	if err != nil {
		return connection, err
	}
	sandbox.cmd = exec.Command(options.binary, getSandboxArgs(options, runDir, helper, openCode)...)
	sandbox.cmd.Env = env
	sandbox.cmd.Stdout = io.MultiWriter(os.Stderr, sandbox.output)
	sandbox.cmd.Stderr = sandbox.cmd.Stdout
	sandbox.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sandbox.cmd.Start(); err != nil {
		return connection, fmt.Errorf("start bubblewrap: %w", err)
	}
	go func() {
		sandbox.err = sandbox.cmd.Wait()
		close(sandbox.done)
	}()

	// Clients reach OpenCode through the helper's server socket.
	listener, err := net.Listen("tcp", net.JoinHostPort(options.BindAddress, "0"))
	if err != nil {
		return connection, err
	}
	sandbox.listeners = append(sandbox.listeners, listener)
	go serveTunnel(listener, dialUnix(filepath.Join(runDir, sandboxServerSocket)))
	host := options.BindAddress
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	connection = Connection{
		URL:       "http://" + net.JoinHostPort(host, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)),
		Password:  options.password,
		Container: options.Name,
		ReadOnly:  options.mounts[0].readOnly,
		Runtime:   options.Runtime,
		Workspace: options.workdir,
	}
	for _, name := range mcpServerNames(options.MCPServers) {
		server := options.MCPServers[name]
		if server.URL == "" {
			continue
		}
		if connection.MCPServers == nil {
			connection.MCPServers = map[string]MCPEndpoint{}
		}
		connection.MCPServers[name] = MCPEndpoint{URL: hostMCPURL(server, ""), Plugin: server.Plugin}
	}

	// A sandbox that exits early ends the wait for it.
	readyCtx, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	go func() {
		select {
		case <-sandbox.done:
			stopWaiting()
		case <-readyCtx.Done():
		}
	}()
	if err := waitReady(readyCtx, connection); err != nil {
		if parent.Err() != nil {
			return connection, err
		}
		output := sandbox.output.String()
		if options.APIKey != "" {
			output = strings.ReplaceAll(output, options.APIKey, "[redacted]")
		}
		select {
		case <-sandbox.done:
			err = fmt.Errorf("OpenCode exited before becoming ready: %v", sandbox.err)
		default:
		}
		return connection, fmt.Errorf("%w\nsandbox output: %s", err, output)
	}
	if err := m.hostMCPError(); err != nil {
		return connection, err
	}
	return connection, nil
}

// check is Check for the bubblewrap runtime.
func (s *sandboxProcess) check() error {
	select {
	case <-s.done:
		return fmt.Errorf("%w: its sandbox exited (%v)", ErrStopped, s.err)
	default:
		return nil
	}
}

// stop ends the sandbox, asking first, and removes its run directory.
func (s *sandboxProcess) stop(ctx context.Context) error {
	for _, listener := range s.listeners {
		listener.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		// bubblewrap's --die-with-parent takes everything in the sandbox down with it.
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-s.done:
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
		select {
		case <-s.done:
		default:
			syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			<-s.done
		}
	}
	return os.RemoveAll(s.runDir)
}

// sandboxRunBase is where run directories go: private to the user, and short, since socket
// paths are limited to about 100 bytes.
func sandboxRunBase() string {
	if directory := os.Getenv("XDG_RUNTIME_DIR"); directory != "" {
		return directory
	}
	return os.TempDir()
}

// defaultSandboxHome is OpenCode's home folder on the host when bob.toml names none.
func defaultSandboxHome() string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "bob", "home")
}

// getSandboxEndpoints gives every endpoint URL a tunnel: a loopback address per host and a port
// per host and port. Ports below 1024 move up by 10000, since the sandbox cannot bind them. It
// returns the tunnels, each URL as OpenCode reaches it in the sandbox, and the /etc/hosts lines
// that send the host names to their loopback addresses, so TLS still checks the real name.
func getSandboxEndpoints(urls []string) ([]sandboxEndpoint, map[string]string, []string, error) {
	var endpoints []sandboxEndpoint
	inside := map[string]string{}
	var hosts []string
	addresses := map[string]string{}
	targets := map[string]string{}
	for _, raw := range urls {
		if _, done := inside[raw]; done {
			continue
		}
		endpoint, err := url.Parse(raw)
		if err != nil {
			return nil, nil, nil, err
		}
		host := endpoint.Hostname()
		ip := net.ParseIP(host)
		if host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil, nil, nil, fmt.Errorf("%s: the sandbox has its own loopback; reach the host as host.containers.internal", raw)
		}
		port, err := strconv.Atoi(endpoint.Port())
		if endpoint.Port() == "" {
			port, err = 80, nil
			if endpoint.Scheme == "https" {
				port = 443
			}
		}
		if err != nil {
			return nil, nil, nil, err
		}
		address, known := addresses[host]
		if !known {
			address = fmt.Sprintf("127.0.0.%d", len(addresses)+2)
			addresses[host] = address
			if ip == nil {
				hosts = append(hosts, address+" "+host)
			}
		}
		local := port
		if local < 1024 {
			local += 10000
		}
		listen := net.JoinHostPort(address, strconv.Itoa(local))
		target := host
		if host == "host.docker.internal" || host == "host.containers.internal" {
			target = "127.0.0.1"
		}
		target = net.JoinHostPort(target, strconv.Itoa(port))
		if previous, taken := targets[listen]; !taken {
			targets[listen] = target
			endpoints = append(endpoints, sandboxEndpoint{
				Listen: listen,
				Socket: fmt.Sprintf("out-%d.sock", len(endpoints)),
				target: target,
			})
		} else if previous != target {
			return nil, nil, nil, fmt.Errorf("%s: its port collides with another endpoint of %s in the sandbox", raw, host)
		}
		name := host
		if ip != nil {
			name = address
		}
		endpoint.Host = net.JoinHostPort(name, strconv.Itoa(local))
		inside[raw] = endpoint.String()
	}
	return endpoints, inside, hosts, nil
}

// getSandboxArgs is the bubblewrap command line. It carries no secrets: they go through the
// environment.
func getSandboxArgs(options resolvedOptions, runDir, helper, openCode string) []string {
	args := []string{
		"--unshare-all", "--die-with-parent", "--new-session", "--hostname", "bob",
		"--ro-bind", "/usr", "/usr",
	}
	// RHEL's /bin, /sbin, /lib and /lib64 point into /usr.
	for _, directory := range []string{"/bin", "/sbin", "/lib", "/lib64"} {
		info, err := os.Lstat(directory)
		switch {
		case err != nil:
		case info.Mode()&os.ModeSymlink != 0:
			if target, err := os.Readlink(directory); err == nil {
				args = append(args, "--symlink", target, directory)
			}
		case info.IsDir():
			args = append(args, "--ro-bind", directory, directory)
		}
	}
	args = append(args,
		"--ro-bind", "/etc", "/etc",
		"--ro-bind", filepath.Join(runDir, "hosts"), "/etc/hosts",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--bind", runDir, sandboxRunDir,
		"--bind", options.Home, sandboxHome,
		"--ro-bind", helper, sandboxBin+"/bob-manager",
		"--ro-bind", openCode, sandboxBin+"/opencode",
	)
	for _, mount := range options.mounts {
		flag := "--bind"
		if mount.readOnly {
			flag = "--ro-bind"
		}
		args = append(args, flag, mount.path, mount.path)
	}
	return append(args, "--chdir", options.workdir, sandboxBin+"/bob-manager", "--sandbox-helper")
}

// getSandboxEnvironment is the sandbox's whole environment: nothing of bob-manager's own, the
// variables OpenCode's configuration reads with the URLs it reaches in the sandbox, and the plan
// for the helper.
func getSandboxEnvironment(options resolvedOptions, inside map[string]string, endpoints []sandboxEndpoint) ([]string, error) {
	sandboxed := options
	sandboxed.BaseURL = inside[options.BaseURL]
	sandboxed.MCPServers = make(map[string]MCPServer, len(options.MCPServers))
	for name, server := range options.MCPServers {
		if server.URL != "" {
			server.URL = inside[server.URL]
		}
		sandboxed.MCPServers[name] = server
	}
	launch, err := getLaunchEnvironment(sandboxed)
	if err != nil {
		return nil, err
	}
	plan, err := json.Marshal(sandboxPlan{
		Endpoints: endpoints,
		Server:    sandboxServer,
		Command:   []string{sandboxBin + "/opencode", "serve", "--hostname", "127.0.0.1", "--port", "4096"},
	})
	if err != nil {
		return nil, err
	}
	name := "opencode"
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + sandboxHome, "USER=" + name, "LANG=" + lang,
		"OPENCODE_CONFIG=" + sandboxRunDir + "/opencode.json", sandboxPlanVariable + "=" + string(plan),
	}
	for _, entry := range launch {
		// The container's firewall and identity settings mean nothing here.
		if !strings.HasPrefix(entry, "BOB_") {
			env = append(env, entry)
		}
	}
	return env, nil
}

// tailWriter keeps the last tailSize bytes written to it.
type tailWriter struct {
	mu   sync.Mutex
	data []byte
}

const tailSize = 64 << 10

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	if len(w.data) > tailSize {
		w.data = w.data[len(w.data)-tailSize:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data)
}
