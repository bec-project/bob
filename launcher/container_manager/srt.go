package containermanager

import (
	"context"
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
	"strconv"
	"strings"
	"syscall"
	"time"
)

// This variant of bob runs OpenCode under Anthropic's sandbox-runtime (srt) instead of driving
// bubblewrap itself. srt builds the bubblewrap sandbox, removes its network, and runs HTTP and
// SOCKS proxies on the host that only let through the domains bob.toml names; inside, socat
// bridges the proxies' Unix sockets to localhost ports and the proxy variables point at them.
// srt runs on OpenCode's own Bun (BUN_BE_BUN), so no Node.js is needed.
//
// bob-manager still adds what srt does not do: the way in for the app (a socket the helper serves
// inside), OpenCode's MCP-only tool rules, the minimal set of readable files, and its own seccomp
// filter, which it stacks onto srt's bubblewrap through a bwrap shim (srt's bwrapPath).

// systemCertificates is RHEL's bundle of the authorities the host trusts.
const systemCertificates = "/etc/pki/tls/certs/ca-bundle.crt"

// srtShimName is the name bob-manager answers to as srt's bwrap.
const srtShimName = "bwrap"

const (
	shimRealVariable    = "BOB_BWRAP_REAL"
	shimSeccompVariable = "BOB_BWRAP_SECCOMP"
)

// srtSettings is srt's settings file (see its README).
type srtSettings struct {
	Network struct {
		AllowedDomains      []string `json:"allowedDomains"`
		DeniedDomains       []string `json:"deniedDomains"`
		AllowAllUnixSockets bool     `json:"allowAllUnixSockets"`
	} `json:"network"`
	Filesystem struct {
		DenyRead   []string `json:"denyRead"`
		AllowRead  []string `json:"allowRead"`
		AllowWrite []string `json:"allowWrite"`
		DenyWrite  []string `json:"denyWrite"`
	} `json:"filesystem"`
	BwrapPath string `json:"bwrapPath"`
	SocatPath string `json:"socatPath"`
	Ripgrep   struct {
		Command string `json:"command"`
	} `json:"ripgrep"`
}

// startSandbox starts OpenCode under srt and waits until it answers.
func (m *Manager) startSandbox(parent context.Context, options resolvedOptions) (connection Connection, startErr error) {
	if !sandboxSupported {
		return connection, errors.New("bob's sandbox runs on Linux only; on macOS, run bob in a Linux VM")
	}
	helper, err := os.Executable()
	if err != nil {
		return connection, err
	}
	if helper, err = filepath.EvalSymlinks(helper); err != nil {
		return connection, err
	}
	openCode, err := findOpenCode(options.OpenCode, helper)
	if err != nil {
		return connection, err
	}
	tools, err := findSandboxTools(options, helper)
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
	allowed, inside, err := getSRTEndpoints(urls)
	if err != nil {
		return connection, err
	}
	runDir, err := os.MkdirTemp(sandboxRunBase(), "bob-")
	if err != nil {
		return connection, err
	}
	sandbox := &sandboxProcess{runDir: runDir, done: make(chan struct{}), output: &tailWriter{}}
	m.sandbox = sandbox
	sockets := filepath.Join(runDir, "sockets")
	if err := os.Mkdir(sockets, 0o700); err != nil {
		return connection, err
	}
	config := filepath.Join(runDir, "opencode.json")
	if err := os.WriteFile(config, openCodeConfig, 0o644); err != nil {
		return connection, err
	}
	filter, err := seccompFilter()
	if err != nil {
		return connection, err
	}
	filterPath := filepath.Join(runDir, "seccomp.bpf")
	if err := os.WriteFile(filterPath, filter, 0o600); err != nil {
		return connection, err
	}
	// srt runs bubblewrap by this name; bob-manager answers it and adds the seccomp filter.
	shimDir := filepath.Join(runDir, "bin")
	if err := os.Mkdir(shimDir, 0o700); err != nil {
		return connection, err
	}
	shim := filepath.Join(shimDir, srtShimName)
	if err := os.Symlink(helper, shim); err != nil {
		return connection, err
	}
	readable, err := getSRTReadable(options, helper, openCode, tools, runDir)
	if err != nil {
		return connection, err
	}
	settings := srtSettings{}
	settings.Network.AllowedDomains = allowed
	settings.Network.DeniedDomains = []string{}
	// The helper serves the app's way in on a Unix socket, which srt's seccomp would refuse.
	settings.Network.AllowAllUnixSockets = true
	settings.Filesystem.DenyRead = []string{"/"}
	settings.Filesystem.AllowRead = readable
	settings.Filesystem.AllowWrite = []string{options.Home, sockets}
	for _, mount := range options.mounts {
		if !mount.readOnly {
			settings.Filesystem.AllowWrite = append(settings.Filesystem.AllowWrite, mount.path)
		}
	}
	settings.Filesystem.DenyWrite = []string{}
	settings.BwrapPath = shim
	settings.SocatPath = tools.socat
	settings.Ripgrep.Command = tools.ripgrep
	settingsPath := filepath.Join(runDir, "srt-settings.json")
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return connection, err
	}
	if err := os.WriteFile(settingsPath, encoded, 0o600); err != nil {
		return connection, err
	}

	env, err := getSRTEnvironment(options, inside, openCode, sockets, config, tools, filterPath)
	if err != nil {
		return connection, err
	}
	// srt runs on the Bun inside OpenCode's executable.
	sandbox.cmd = exec.Command(openCode, tools.srt, "--settings", settingsPath, "--", helper, "--sandbox-helper")
	sandbox.cmd.Dir = options.workdir
	sandbox.cmd.Env = env
	sandbox.cmd.Stdout = io.MultiWriter(os.Stderr, sandbox.output)
	sandbox.cmd.Stderr = sandbox.cmd.Stdout
	sandbox.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sandbox.cmd.Start(); err != nil {
		return connection, fmt.Errorf("start sandbox-runtime: %w", err)
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
	go serveTunnel(listener, dialUnix(filepath.Join(sockets, sandboxServerSocket)))
	return m.finishStart(ctx, parent, options, sandbox, listener)
}

// sandboxTools are the programs bob-manager runs srt with.
type sandboxTools struct {
	srt     string
	bwrap   string
	socat   string
	bash    string
	ripgrep string
}

// findSandboxTools finds srt (sandbox.runtime, else the copy installed with bob-manager in
// lib/sandbox-runtime/node_modules), bwrap, socat, bash, which srt runs its bridges with, and ripgrep, which
// srt scans the working directory with (installed with bob-manager in libexec, else on PATH).
func findSandboxTools(options resolvedOptions, helper string) (sandboxTools, error) {
	var tools sandboxTools
	prefix := filepath.Dir(filepath.Dir(helper))
	tools.srt = options.SandboxRuntime
	if tools.srt == "" {
		tools.srt = filepath.Join(prefix, "lib", "sandbox-runtime", "node_modules", "@anthropic-ai", "sandbox-runtime", "dist", "cli.js")
	}
	if _, err := os.Stat(tools.srt); err != nil {
		return tools, fmt.Errorf("find sandbox-runtime (sandbox.runtime): %w", err)
	}
	var err error
	tools.bwrap = options.binary
	if tools.socat, err = exec.LookPath("socat"); err != nil {
		return tools, fmt.Errorf("find socat on PATH; install socat: %w", err)
	}
	if tools.bash, err = exec.LookPath("bash"); err != nil {
		return tools, err
	}
	tools.ripgrep = filepath.Join(prefix, "libexec", "rg")
	if _, err := os.Stat(tools.ripgrep); err != nil {
		if tools.ripgrep, err = exec.LookPath("rg"); err != nil {
			return tools, fmt.Errorf("find ripgrep (rg), which sandbox-runtime needs: %w", err)
		}
	}
	for _, tool := range []*string{&tools.socat, &tools.bash, &tools.ripgrep} {
		if *tool, err = filepath.EvalSymlinks(*tool); err != nil {
			return tools, err
		}
	}
	return tools, nil
}

// getSRTReadable lists what the sandbox may read: the programs it runs (OpenCode, bob-manager,
// bash and socat for srt's bridges), the system's library folders (srt binds the paths it is
// given after resolving their symlinks, so the loader's names for single libraries would be
// missing), the certificate authorities, the hosts file (srt's bridges dial "localhost"), bob's
// home and sockets, OpenCode's configuration and the workspace mounts.
func getSRTReadable(options resolvedOptions, helper, openCode string, tools sandboxTools, runDir string) ([]string, error) {
	readable := []string{helper, openCode, tools.bash, tools.socat}
	for _, path := range []string{"/usr/lib64", "/usr/lib", "/etc/pki", "/etc/ssl", "/etc/hosts"} {
		if _, err := os.Stat(path); err == nil {
			readable = append(readable, path)
		}
	}
	readable = append(readable, options.Home, filepath.Join(runDir, "sockets"), filepath.Join(runDir, "opencode.json"))
	for _, mount := range options.mounts {
		readable = append(readable, mount.path)
	}
	return readable, nil
}

// getSRTEndpoints lists the domains srt's proxies let through, with their ports, and each URL as
// OpenCode reaches it. host.containers.internal means this machine: the proxy dials it as
// 127.0.0.1, which srt allows only as an explicit literal.
func getSRTEndpoints(urls []string) ([]string, map[string]string, error) {
	var allowed []string
	inside := map[string]string{}
	for _, raw := range urls {
		if _, done := inside[raw]; done {
			continue
		}
		endpoint, err := url.Parse(raw)
		if err != nil {
			return nil, nil, err
		}
		host := endpoint.Hostname()
		ip := net.ParseIP(host)
		if host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil, nil, fmt.Errorf("%s: the sandbox has its own loopback; reach the host as host.containers.internal", raw)
		}
		port := endpoint.Port()
		if port == "" {
			port = "80"
			if endpoint.Scheme == "https" {
				port = "443"
			}
		}
		if host == "host.docker.internal" || host == "host.containers.internal" {
			host = "127.0.0.1"
			endpoint.Host = net.JoinHostPort(host, port)
		}
		entry := net.JoinHostPort(host, port)
		if ip := net.ParseIP(host); ip == nil || ip.To4() != nil {
			entry = host + ":" + port
		}
		allowed = append(allowed, entry)
		inside[raw] = endpoint.String()
	}
	return allowed, inside, nil
}

// getSRTEnvironment is srt's whole environment, which srt passes on into the sandbox (adding its
// proxy variables): nothing of bob-manager's own, the variables OpenCode's configuration reads
// with the URLs it reaches, the plan for the helper, and the shim's settings.
func getSRTEnvironment(options resolvedOptions, inside map[string]string, openCode, sockets, config string, tools sandboxTools, filterPath string) ([]string, error) {
	sandboxed := options
	sandboxed.BaseURL = inside[options.BaseURL]
	sandboxed.MCPServers = make(map[string]MCPServer, len(options.MCPServers))
	for name, server := range options.MCPServers {
		if server.URL != "" {
			server.URL = inside[server.URL]
		}
		sandboxed.MCPServers[name] = server
	}
	launch, err := getOpenCodeEnvironment(sandboxed)
	if err != nil {
		return nil, err
	}
	plan, err := json.Marshal(sandboxPlan{
		Server:  sandboxServer,
		Sockets: sockets,
		Command: []string{openCode, "serve", "--hostname", "127.0.0.1", "--port", "4096"},
		// srt's runner, the shim's settings and srt's proxy exceptions stay out of OpenCode's
		// environment: without NO_PROXY, OpenCode reaches 127.0.0.1 through the proxy too, which
		// is how it reaches a host MCP server.
		Unset: []string{"BUN_BE_BUN", shimRealVariable, shimSeccompVariable, "NO_PROXY", "no_proxy"},
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
		"PATH=" + filepath.Dir(tools.bash) + ":" + filepath.Dir(tools.socat) + ":/usr/bin:/bin",
		"HOME=" + options.Home, "USER=" + name, "LANG=" + lang, "TMPDIR=" + filepath.Join(options.Home, ".cache"),
		"OPENCODE_CONFIG=" + config, sandboxPlanVariable + "=" + string(plan),
		"BUN_BE_BUN=1", shimRealVariable + "=" + tools.bwrap, shimSeccompVariable + "=" + filterPath,
		"XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
	}
	if _, err := os.Stat(systemCertificates); err == nil {
		env = append(env, "NODE_EXTRA_CA_CERTS="+systemCertificates)
	}
	return append(env, launch...), nil
}

// finishStart waits for OpenCode behind the app's way in and returns the connection.
func (m *Manager) finishStart(ctx, parent context.Context, options resolvedOptions, sandbox *sandboxProcess, listener net.Listener) (Connection, error) {
	host := options.BindAddress
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	connection := Connection{
		URL:       "http://" + net.JoinHostPort(host, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)),
		Password:  options.password,
		Container: options.name,
		ReadOnly:  len(options.mounts) > 0 && options.mounts[0].readOnly,
		Runtime:   runtimeName,
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
		connection.MCPServers[name] = MCPEndpoint{URL: hostMCPURL(server), Plugin: server.Plugin}
	}
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
