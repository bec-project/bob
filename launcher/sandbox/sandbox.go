package sandbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// bob runs OpenCode on the host under Anthropic's sandbox-runtime (srt.go), which builds a
// bubblewrap sandbox with its own namespaces and no network. It needs no kernel modules or root
// access.

// runtimeName is what Connection.Runtime reports, and the app shows beside the sandbox's name.
const runtimeName = "srt"

// sandboxSupported is false where srt does not run; tests switch it on.
var sandboxSupported = runtime.GOOS == "linux" || runtime.GOOS == "darwin"

// openCodeProvider is the provider name defined in opencode.json.
const openCodeProvider = "self-hosted"

const (
	// sandboxServer is OpenCode's address in the sandbox.
	sandboxServer = "127.0.0.1:4096"
)

// openCodeConfig is OpenCode's base configuration: the self-hosted provider, read from the
// environment.
//
//go:embed opencode.json
var openCodeConfig []byte

// sandboxProcess is a running sandbox and the app's way in that bob-manager serves for it.
type sandboxProcess struct {
	cmd       *exec.Cmd
	done      chan struct{}
	err       error // Written before done closes; read only after receiving from done.
	output    *tailWriter
	runDir    string
	listeners []net.Listener
}

// check is Check for the sandbox.
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
	running := s.cmd != nil && s.cmd.Process != nil
	if running {
		select {
		case <-s.done:
			// Reaped: its process group ID may already belong to someone else.
			running = false
		default:
		}
	}
	if running {
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

// findOpenCode is sandbox.opencode, looked up on PATH when it is a bare name, or else the OpenCode
// installed with bob-manager (libexec beside its bin, as the RPM has it), or else opencode on PATH.
func findOpenCode(configured, helper string) (string, error) {
	if configured == "" {
		bundled := filepath.Join(filepath.Dir(filepath.Dir(helper)), "libexec", "opencode")
		if info, err := os.Stat(bundled); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return bundled, nil
		}
		configured = "opencode"
	}
	openCode, err := exec.LookPath(configured)
	if err != nil {
		return "", fmt.Errorf("find OpenCode (sandbox.opencode): %w", err)
	}
	return filepath.Abs(openCode)
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

// getOpenCodeEnvironment is what OpenCode's configuration reads: the provider, the model, the
// server's password, and the MCP servers and model limit merged into opencode.json.
func getOpenCodeEnvironment(options resolvedOptions) ([]string, error) {
	environment := []string{
		"API_KEY=" + options.APIKey, "LLM_BASE_URL=" + options.BaseURL,
		"LLM_MODEL_ID=" + options.Model, "OPENCODE_SERVER_PASSWORD=" + options.password,
	}
	// OpenCode may use the MCP servers' tools and nothing else of its own: no shell, no files, no
	// web. Rules are read in order and the last match wins; JSON objects keep sorted keys, so "*"
	// comes first.
	permission := map[string]any{"*": "deny"}
	config := map[string]any{"permission": permission}
	mcp := map[string]any{}
	for _, name := range mcpServerNames(options.MCPServers) {
		server := options.MCPServers[name]
		mcp[name] = map[string]any{"type": "remote", "url": server.URL, "oauth": false, "enabled": true}
		permission[name+"_*"] = "allow"
	}
	if len(mcp) > 0 {
		config["mcp"] = mcp
	}
	if contextLimit, outputLimit := options.modelLimit(); contextLimit > 0 {
		// OpenCode deep-merges this into opencode.json, whose model key is the model ID.
		config["provider"] = map[string]any{openCodeProvider: map[string]any{"models": map[string]any{
			options.Model: map[string]any{"limit": map[string]any{"context": contextLimit, "output": outputLimit}},
		}}}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return append(environment, "OPENCODE_CONFIG_CONTENT="+string(encoded)), nil
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
