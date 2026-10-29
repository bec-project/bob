package sandbox

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHelperRefusesAWayInOfTheHostsNetwork(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	file, err := listener.(*net.TCPListener).File()
	listener.Close()
	if err != nil {
		t.Fatal(err)
	}
	stdin := os.Stdin
	os.Stdin = file
	defer func() { os.Stdin = stdin }()
	if err := RunSandboxHelper([]string{"true"}); err == nil || !strings.Contains(err.Error(), "must be a Unix socket") {
		t.Fatalf("RunSandboxHelper with a TCP socket = %v", err)
	}
}

func TestSRTEndpointsAllowEachHostAndPort(t *testing.T) {
	stubHostLoopbackName(t)
	allowed, inside, err := getSRTEndpoints([]string{
		"https://ai.example/api",
		"http://localhost:8020/mcp",
		"http://127.0.0.1:8021/mcp",
		"http://192.0.2.7:9000/mcp",
		"https://ai.example/api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowed, []string{"ai.example:443", "127.0.0.1:8020", "localhost4.localdomain4:8020", "127.0.0.1:8021", "localhost4.localdomain4:8021", "192.0.2.7:9000"}) {
		t.Fatalf("allowed %v", allowed)
	}
	// OpenCode reaches this machine by a name its NO_PROXY does not keep from the proxy; other
	// URLs stay as they are.
	if inside["http://localhost:8020/mcp"] != "http://localhost4.localdomain4:8020/mcp" || inside["http://127.0.0.1:8021/mcp"] != "http://localhost4.localdomain4:8021/mcp" || inside["https://ai.example/api"] != "https://ai.example/api" {
		t.Fatalf("inside %v", inside)
	}
	for _, raw := range []string{"http://127.0.0.2:8000/mcp", "http://[::1]:8000/mcp", "http://localhost4.localdomain4:8000/mcp"} {
		if _, _, err := getSRTEndpoints([]string{raw}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	lookupHost = func(string) ([]string, error) { return []string{"127.0.0.1", "::1"}, nil }
	if _, _, err := getSRTEndpoints([]string{"http://localhost:8020/mcp"}); err == nil {
		t.Fatal("accepted a loopback name that also resolves to ::1")
	}
}

func TestSRTEnvironmentKeepsSRTsTemporaryFolderInBobsHome(t *testing.T) {
	stubHostLoopbackName(t)
	options := sandboxTestOptions(t)
	_, inside, err := getSRTEndpoints([]string{options.BaseURL, options.MCPServers["bec"].URL})
	if err != nil {
		t.Fatal(err)
	}
	tools := sandboxTools{bash: "/usr/bin/bash", socat: "/usr/bin/socat", bwrap: "/usr/bin/bwrap"}
	policy := openCodePolicy{directory: "/run/bob-1/policy", home: "/run/bob-1/policy/home", config: "/run/bob-1/policy/config/opencode/opencode.json"}
	env, err := getSRTEnvironment(options, inside, policy, tools, "/data/bob/home/.cache/tmp")
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		settings[key] = value
	}
	if settings["TMPDIR"] != "/data/bob/home/.cache/tmp" || settings["CLAUDE_CODE_TMPDIR"] != settings["TMPDIR"] || settings["HOME"] != policy.home {
		t.Fatalf("srt's temporary folder or home: %v", settings)
	}
	if settings["LLM_BASE_URL"] != options.BaseURL || settings["OPENCODE_CONFIG"] != policy.config {
		t.Fatal("OpenCode's provider URL or configuration changed")
	}
	if !strings.Contains(settings["OPENCODE_CONFIG_CONTENT"], `"url":"http://localhost4.localdomain4:8020/mcp"`) {
		t.Fatalf("MCP configuration %s", settings["OPENCODE_CONFIG_CONTENT"])
	}
	for _, name := range []string{"NO_PROXY", "BUN_BE_BUN", "HTTP_PROXY"} {
		if _, set := settings[name]; set {
			t.Errorf("srt gets %s from bob-manager", name)
		}
	}
}

func TestStartRefusesMissingSeccompBeforeStartingAnyProcess(t *testing.T) {
	options, root := fakeSandbox(t)
	helper, err := validateSRTSeccompHelper(options.SandboxRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "host-mcp-started")
	options.MCPServers = map[string]MCPServer{"bec": {
		URL:         "http://localhost:19000/mcp",
		HostCommand: []string{"/bin/sh", "-c", "touch \"$1\"", "sh", marker},
	}}
	manager := &Manager{}
	_, err = manager.Start(context.Background(), options)
	if err == nil || !strings.Contains(err.Error(), "reinstall the complete runtime") {
		t.Fatalf("missing-helper startup = %v", err)
	}
	for _, path := range []string{marker, filepath.Join(root, "args")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed preflight started a process: %s, %v", path, err)
		}
	}
	if manager.sandbox != nil || len(manager.hostMCPs) != 0 {
		t.Fatal("failed preflight retained a sandbox or host MCP process")
	}
}

func TestStartRefusesARunFolderTooLongForTheWayIn(t *testing.T) {
	options, root := fakeSandbox(t)
	deep := filepath.Join(root, strings.Repeat("r", 100))
	if err := os.Mkdir(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", deep)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	manager := &Manager{}
	_, err := manager.Start(ctx, options)
	if err == nil || !strings.Contains(err.Error(), "too long for a Unix socket") || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("Start = %v", err)
	}
	if entries, _ := os.ReadDir(deep); len(entries) != 0 {
		t.Fatalf("a failed start left %v behind", entries)
	}
}
