package sandbox

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

// This variant of bob runs OpenCode under Anthropic's sandbox-runtime (srt) the way srt is meant
// to be used: srt's command line on Node.js, its settings file, its seccomp filter and its
// proxies. srt builds the bubblewrap sandbox, removes its network and runs HTTP and SOCKS proxies
// on the host that only let through the domains bob.toml names; inside, socat bridges the
// proxies' Unix sockets to localhost ports and the proxy variables point at them. srt's filter
// refuses creating Unix sockets and new namespaces in the sandbox.
//
// bob-manager adds only what srt has no setting for: the app's way in (see tunnel.go) and
// OpenCode's MCP-only tool rules.

// systemCertificates is RHEL's bundle of the authorities the host trusts.
const systemCertificates = "/etc/pki/tls/certs/ca-bundle.crt"

// hostLoopbackName is the name OpenCode reaches this machine's loopback by: srt sets NO_PROXY so
// that "localhost" and 127.0.0.1 skip its proxy and stay in the sandbox, where nothing listens.
// RHEL's /etc/hosts maps localhost4.localdomain4 to 127.0.0.1 alone, and srt's domain list takes
// only names with a dot; srt's proxy resolves it there and lets it through because the literal
// address and port are on the list too (srt's README: "Resolved-address check").
const hostLoopbackName = "localhost4.localdomain4"

// srtSettings is srt's settings file (see its README).
type srtSettings struct {
	Network struct {
		AllowedDomains  []string `json:"allowedDomains"`
		DeniedDomains   []string `json:"deniedDomains"`
		StrictAllowlist bool     `json:"strictAllowlist"`
		// AllowLocalBinding lets the sandbox bind and reach loopback ports; macOS only.
		AllowLocalBinding bool `json:"allowLocalBinding,omitempty"`
	} `json:"network"`
	Filesystem struct {
		DenyRead   []string `json:"denyRead"`
		AllowRead  []string `json:"allowRead"`
		AllowWrite []string `json:"allowWrite"`
		DenyWrite  []string `json:"denyWrite"`
	} `json:"filesystem"`
	BwrapPath string      `json:"bwrapPath,omitempty"`
	SocatPath string      `json:"socatPath,omitempty"`
	Seccomp   *srtSeccomp `json:"seccomp,omitempty"`
	Ripgrep   struct {
		Command string `json:"command"`
	} `json:"ripgrep"`
}

// srtSeccomp names srt's seccomp helper (Linux only).
type srtSeccomp struct {
	ApplyPath string `json:"applyPath"`
}

// sandboxPath is the sandbox's PATH: the folders of the programs srt runs in it, then the
// system's.
func sandboxPath(tools sandboxTools) string {
	var folders []string
	for _, program := range []string{tools.bash, tools.socat} {
		if program != "" {
			folders = append(folders, filepath.Dir(program))
		}
	}
	return strings.Join(append(folders, "/usr/bin", "/bin"), ":")
}

// startSandbox starts OpenCode under srt and waits until it answers.
func (m *Manager) startSandbox(parent context.Context, options resolvedOptions) (connection Connection, startErr error) {
	if !sandboxSupported {
		return connection, errors.New("bob's sandbox runs on Linux and macOS only")
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
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
	options.Home, err = prepareSandboxData(options.Home)
	if err != nil {
		return connection, err
	}
	if err := validateHostMCPStorage(options); err != nil {
		return connection, err
	}
	temporary := filepath.Join(options.Home, ".cache", "tmp")
	if err := os.MkdirAll(temporary, 0o700); err != nil {
		return connection, fmt.Errorf("create bob's home folder: %w", err)
	}
	tools, err := findSandboxTools(ctx, options, helper)
	if err != nil {
		return connection, err
	}
	if err := validateSandboxExecutables(options, helper, openCode, tools); err != nil {
		return connection, err
	}
	if err := validateOpenCodeVersion(ctx, openCode); err != nil {
		return connection, err
	}
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
	policy, err := prepareOpenCodePolicy(options.Home)
	if err != nil {
		return connection, err
	}
	settings := srtSettings{}
	settings.Network.AllowedDomains = allowed
	settings.Network.DeniedDomains = []string{}
	settings.Network.StrictAllowlist = true
	// Nothing of the host is readable but what is listed: srt documents denyRead ["/"] as hiding
	// every directory in / (but /proc, /dev and /sys), with allowRead opening paths again.
	settings.Filesystem.DenyRead = []string{"/"}
	settings.Filesystem.AllowRead = getSRTReadable(options, helper, openCode, tools, policy.directory)
	settings.Filesystem.AllowWrite = sandboxDataPaths(options.Home)
	settings.Filesystem.DenyWrite = []string{policy.directory}
	settings.Ripgrep.Command = tools.ripgrep
	applyPlatformSettings(&settings, tools)
	settingsPath := filepath.Join(runDir, "srt-settings.json")
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return connection, err
	}
	if err := os.WriteFile(settingsPath, encoded, 0o600); err != nil {
		return connection, err
	}
	env, err := getSRTEnvironment(options, inside, policy, tools, temporary)
	if err != nil {
		return connection, err
	}

	stdin, listener, port, err := openWayIn(runDir, options.BindAddress)
	if err != nil {
		return connection, err
	}
	if stdin != nil {
		defer stdin.Close()
	}
	sandbox.listeners = append(sandbox.listeners, listener)
	sandbox.cmd = exec.Command(tools.node, tools.srt, "--settings", settingsPath, "--",
		helper, "--sandbox-helper", openCode, "serve", "--hostname", "127.0.0.1", "--port", port)
	sandbox.cmd.Dir = policy.home
	sandbox.cmd.Env = env
	if stdin != nil {
		sandbox.cmd.Stdin = stdin
	}
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
	return m.finishStart(ctx, parent, options, sandbox, listener.Addr().(*net.TCPAddr).Port)
}

// sandboxTools are the programs bob-manager runs srt with.
type sandboxTools struct {
	node         string
	srt          string
	bwrap        string
	socat        string
	bash         string
	ripgrep      string
	applySeccomp string
}

// findSandboxTools finds Node.js (installed with bob-manager in libexec, else on PATH), srt
// (sandbox.runtime, else the copy installed with bob-manager in lib/sandbox-runtime/node_modules),
// bash, which srt runs the command with, ripgrep, which srt scans the writable folders with
// (installed with bob-manager in libexec, else on PATH), and what srt needs on this platform
// beside them (findPlatformTools).
func findSandboxTools(ctx context.Context, options resolvedOptions, helper string) (sandboxTools, error) {
	var tools sandboxTools
	prefix := filepath.Dir(filepath.Dir(helper))
	var err error
	// The macOS app carries its own Node.js; RHEL has it as a system package.
	tools.node = filepath.Join(prefix, "libexec", "node")
	if _, err := os.Stat(tools.node); err != nil {
		if tools.node, err = exec.LookPath("node"); err != nil {
			return tools, fmt.Errorf("find Node.js (node) on PATH, which sandbox-runtime runs on; install Node.js 22.12 or later: %w", err)
		}
	}
	tools.srt = options.SandboxRuntime
	if tools.srt == "" {
		tools.srt = filepath.Join(prefix, "lib", "sandbox-runtime", "node_modules", "@anthropic-ai", "sandbox-runtime", "dist", "cli.js")
	}
	if _, err := os.Stat(tools.srt); err != nil {
		return tools, fmt.Errorf("find sandbox-runtime (sandbox.runtime): %w", err)
	}
	tools.bwrap = options.binary
	if err := findPlatformTools(&tools); err != nil {
		return tools, err
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
	if err := validateSandboxExecutables(options, helper, "", tools); err != nil {
		return tools, err
	}
	for _, tool := range []*string{&tools.node, &tools.socat, &tools.bash, &tools.ripgrep} {
		if *tool == "" {
			continue
		}
		if *tool, err = filepath.EvalSymlinks(*tool); err != nil {
			return tools, err
		}
	}
	if err := validateNodeVersion(ctx, tools.node); err != nil {
		return tools, err
	}
	return tools, nil
}

// getSRTReadable lists what the sandbox may read: the programs it runs (bob-manager, OpenCode,
// and bash and, on Linux, socat for srt's bridges), the system paths this platform needs
// (platformReadable), persistent data/cache/state, and the read-only OpenCode policy.
func getSRTReadable(options resolvedOptions, helper, openCode string, tools sandboxTools, policyDirectory string) []string {
	var readable []string
	for _, program := range []string{helper, openCode, tools.bash, tools.socat} {
		if program != "" {
			readable = append(readable, program)
		}
	}
	for _, path := range platformReadable(tools) {
		if _, err := os.Stat(path); err == nil {
			readable = append(readable, path)
		}
	}
	readable = append(readable, sandboxDataPaths(options.Home)...)
	readable = append(readable, policyDirectory)
	return readable
}

// getSRTEndpoints lists the domains srt's proxies let through, with their ports, and each URL as
// OpenCode reaches it. localhost and 127.0.0.1 mean this machine, which OpenCode reaches as
// thisMachineEndpoint says for this platform.
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
		if !isThisMachine(host) && (host == hostLoopbackName || (ip != nil && ip.IsLoopback())) {
			return nil, nil, fmt.Errorf("%s: name this machine as localhost or 127.0.0.1", raw)
		}
		port := endpoint.Port()
		if port == "" {
			port = "80"
			if endpoint.Scheme == "https" {
				port = "443"
			}
		}
		if isThisMachine(host) {
			var extra []string
			if host, extra, err = thisMachineEndpoint(port); err != nil {
				return nil, nil, err
			}
			endpoint.Host = net.JoinHostPort(host, port)
			allowed = append(allowed, extra...)
		}
		entry := host + ":" + port
		if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
			entry = net.JoinHostPort(host, port)
		}
		allowed = append(allowed, entry)
		inside[raw] = endpoint.String()
	}
	return allowed, inside, nil
}

// lookupHost resolves a name as this machine does; tests replace it.
var lookupHost = net.LookupHost

// checkHostLoopbackName makes sure this machine resolves hostLoopbackName to 127.0.0.1 alone, as
// srt's proxy will.
func checkHostLoopbackName() error {
	addresses, err := lookupHost(hostLoopbackName)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("an endpoint on this machine needs %s in /etc/hosts, as RHEL has it (127.0.0.1 %s): %v", hostLoopbackName, hostLoopbackName, err)
	}
	for _, address := range addresses {
		if address != "127.0.0.1" {
			return fmt.Errorf("%s resolves to %s; an endpoint on this machine needs it to be 127.0.0.1 alone", hostLoopbackName, address)
		}
	}
	return nil
}

// getSRTEnvironment is srt's whole environment, which srt passes on into the sandbox (adding its
// proxy variables): nothing of bob-manager's own, srt's temporary folders in bob's home, and the
// variables OpenCode's configuration reads with the URLs it reaches. srt makes its proxies'
// sockets in TMPDIR, which must stay visible in the sandbox (denyRead hides /tmp), and gives the
// sandbox CLAUDE_CODE_TMPDIR rather than /tmp/claude, which users of one machine would share.
func getSRTEnvironment(options resolvedOptions, inside map[string]string, policy openCodePolicy, tools sandboxTools, temporary string) ([]string, error) {
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
	name := "opencode"
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	env := []string{
		"PATH=" + sandboxPath(tools),
		"USER=" + name, "LANG=" + lang,
		"TMPDIR=" + temporary, "CLAUDE_CODE_TMPDIR=" + temporary,
		"XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
	}
	env = append(env, policy.environment(options.Home)...)
	if _, err := os.Stat(systemCertificates); err == nil {
		env = append(env, "NODE_EXTRA_CA_CERTS="+systemCertificates)
	}
	return append(env, launch...), nil
}

// finishStart waits for OpenCode behind the app's way in, on port, and returns the connection.
func (m *Manager) finishStart(ctx, parent context.Context, options resolvedOptions, sandbox *sandboxProcess, port int) (Connection, error) {
	host := options.BindAddress
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	connection := Connection{
		URL:      "http://" + net.JoinHostPort(host, strconv.Itoa(port)),
		Password: options.password,
		Name:     options.name,
		Runtime:  runtimeName,
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
	if err := verifyOpenCodePolicy(readyCtx, connection, options); err != nil {
		return connection, err
	}
	return connection, nil
}
