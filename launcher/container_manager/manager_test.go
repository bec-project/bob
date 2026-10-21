package containermanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This optional integration test exercises the actual runtime and kernel mount permissions.
func TestContainerIntegration(t *testing.T) {
	if os.Getenv("BOB_CONTAINER_TEST") != "1" {
		t.Skip("set BOB_CONTAINER_TEST=1 to run against Docker")
	}
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mcp" {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost {
				var request struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params struct {
						ProtocolVersion string `json:"protocolVersion"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, "invalid MCP request", http.StatusBadRequest)
					return
				}
				if len(request.ID) == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				result := map[string]any{}
				switch request.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": request.Params.ProtocolVersion,
						"capabilities": map[string]any{"tools": map[string]any{}},
						"serverInfo":   map[string]any{"name": "bob-test", "version": "1"}}
				case "tools/list":
					result["tools"] = []any{}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
				return
			}
			fmt.Fprint(w, `{"synthetic":"mcp"}`)
			return
		}
		if r.URL.Path != "/agent/api/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-provider-token" {
			t.Error("agent did not preserve the configured upstream path and token")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"synthetic\":true}\n\ndata: [DONE]\n\n")
	}))
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream.Listener = listener
	upstream.Start()
	defer upstream.Close()
	baseURL := fmt.Sprintf("http://host.docker.internal:%d/agent/api/", listener.Addr().(*net.TCPAddr).Port)
	blocked := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("reached forbidden host port") }))
	blockedListener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	blocked.Listener = blockedListener
	blocked.Start()
	defer blocked.Close()
	workspace, reference := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(reference, "notes.txt"), []byte("synthetic reference"), 0644); err != nil {
		t.Fatal(err)
	}
	// The reference must be writable by Unix permissions so only :ro denies writes.
	for _, directory := range []string{workspace, reference} {
		if err := os.Chmod(directory, 0777); err != nil {
			t.Fatal(err)
		}
	}
	volume := fmt.Sprintf("bob-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		if output, err := exec.Command("docker", "volume", "rm", volume).CombinedOutput(); err != nil {
			t.Errorf("remove test volume: %s %v", output, err)
		}
	})
	manager := &Manager{}
	t.Cleanup(func() {
		if err := manager.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	connection, err := manager.Start(ctx, Options{
		Runtime:    "docker",
		HomeVolume: volume,
		Workdir:    workspace,
		ReadOnly:   []string{reference},
		BaseURL:    baseURL,
		Model:      "demo-model",
		APIKey:     "synthetic-provider-token",
		MCPServers: map[string]MCPServer{"bob": {Port: 19000, URL: fmt.Sprintf("http://host.docker.internal:%d/mcp", listener.Addr().(*net.TCPAddr).Port)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.MCPPorts["bob"] == "" || connection.Password == "" {
		t.Fatal("missing dynamic MCP port or server authentication")
	}
	binary, err := getExecutable("docker")
	if err != nil {
		t.Fatal(err)
	}
	if connection.Workspace != workspace {
		t.Fatalf("workspace path changed: %q", connection.Workspace)
	}
	check := `test "$PWD" = "$1" || exit 1; touch "$1/probe" || exit 1; rm "$1/probe" || exit 1; cat "$2/notes.txt" || exit 1; if touch "$2/probe" 2>/tmp/error; then exit 1; fi; grep -q 'Read-only file system' /tmp/error`
	agentShell := `exec setpriv --reuid="$BOB_UID" --regid="$BOB_GID" --clear-groups --bounding-set=-all --inh-caps=-all --ambient-caps=-all --no-new-privs sh -c "$@"`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "sh", "-c", agentShell, "agent-check", check, "mount-check", workspace, reference).CombinedOutput(); err != nil {
		t.Fatalf("mount isolation failed: %s", output)
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", connection.URL+"/config", nil)
	req.SetBasicAuth("opencode", connection.Password)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var config struct {
		Model   string   `json:"model"`
		Enabled []string `json:"enabled_providers"`
		MCP     map[string]struct {
			Type    string `json:"type"`
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	if config.Model != "self-hosted/demo-model" || len(config.Enabled) != 1 || config.Enabled[0] != "self-hosted" {
		t.Fatal("backend did not load the restricted provider configuration")
	}
	if config.MCP["bob"].Type != "remote" || !config.MCP["bob"].Enabled || config.MCP["bob"].URL == "" {
		t.Fatal("remote MCP endpoint was not configured")
	}
	req, _ = http.NewRequestWithContext(ctx, "GET", connection.URL+"/mcp", nil)
	req.SetBasicAuth("opencode", connection.Password)
	mcpResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]struct {
		Status string `json:"status"`
	}
	err = json.NewDecoder(mcpResponse.Body).Decode(&status)
	mcpResponse.Body.Close()
	if err != nil || status["bob"].Status != "connected" {
		t.Fatalf("MCP did not start and connect: %v, %v", status, err)
	}
	output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "printenv", "API_KEY").Output()
	if err != nil || string(output) != "synthetic-provider-token\n" {
		t.Fatal("agent did not receive the configured token")
	}
	check = `import os, urllib.request
r = urllib.request.Request(os.environ["LLM_BASE_URL"]+"chat/completions", data=b'{"model":"demo-model","stream":true}', headers={"Content-Type":"application/json", "Authorization":"Bearer "+os.environ["API_KEY"]})
assert b"[DONE]" in urllib.request.urlopen(r).read()
`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "python3", "-c", check).CombinedOutput(); err != nil {
		t.Fatalf("provider connection failed: %s", output)
	}
	check = `import socket
# An allowed IP must still reject a different port.
import urllib.parse, os
host = urllib.parse.urlsplit(os.environ["LLM_BASE_URL"]).hostname
ip = socket.gethostbyname(host)
for target in [(ip, int(os.environ["BOB_TEST_BLOCKED_PORT"])), ("1.1.1.1",443), ("2606:4700:4700::1111",443)]:
    try:
        socket.create_connection(target, timeout=1).close()
    except OSError:
        pass
    else:
        raise AssertionError("unexpected outgoing connection: "+str(target))
# Docker's DNS service must not remain an unrestricted outgoing channel.
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(1)
try:
    s.sendto(bytes.fromhex("12340100000100000000000006676f6f676c6503636f6d0000010001"), ("127.0.0.11",53))
    s.recv(512)
except OSError:
    pass
else:
    raise AssertionError("DNS remained accessible")

`
	if output, err := exec.CommandContext(ctx, binary, "exec", "--env", fmt.Sprintf("BOB_TEST_BLOCKED_PORT=%d", blocked.Listener.Addr().(*net.TCPAddr).Port), connection.Container, "python3", "-c", check).CombinedOutput(); err != nil {
		t.Fatalf("external network isolation failed: %s", output)
	}
	// Inspect PID 1 to verify OpenCode no longer holds bootstrap capabilities.
	check = `import json, urllib.request
r = urllib.request.Request("http://127.0.0.1:4096/global/health")
# PID 1 runs OpenCode after setpriv; all capability sets must be empty.
status = open("/proc/1/status").read()
for line in status.splitlines():
    if line.startswith(("CapInh:", "CapPrm:", "CapEff:", "CapBnd:", "CapAmb:")):
        assert int(line.split()[1], 16) == 0, line
assert "NoNewPrivs:\t1" in status
assert "Seccomp:\t2" in status
`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "python3", "-c", check).CombinedOutput(); err != nil {
		t.Fatalf("agent retained bootstrap privileges: %s", output)
	}
	// Engine exec defaults to the bootstrap identity and can receive engine
	// capabilities. Explicitly recreate the agent's dropped privileges before
	// probing its ability to change the firewall or regain root.
	check = `import ctypes, errno, os, subprocess
status = open("/proc/self/status").read()
for line in status.splitlines():
    if line.startswith(("CapInh:", "CapPrm:", "CapEff:", "CapBnd:", "CapAmb:")):
        assert int(line.split()[1], 16) == 0, line
assert "NoNewPrivs:\t1" in status
assert "Seccomp:\t2" in status
libc = ctypes.CDLL(None, use_errno=True)
# With zero flags this does not change namespaces or require capabilities.
# The default seccomp profile still denies it, proving syscall filtering.
assert libc.unshare(0) == -1 and ctypes.get_errno() == errno.EPERM
for tool in ("iptables", "ip6tables"):
    result = subprocess.run([tool, "-P", "OUTPUT", "ACCEPT"], capture_output=True)
    assert result.returncode != 0, "agent changed firewall"
result = subprocess.run(["setpriv", "--inh-caps=+net_admin", "true"], capture_output=True)
assert result.returncode != 0, "agent regained NET_ADMIN"
if os.getuid() != 0:
    try:
        os.setuid(0)
    except PermissionError:
        pass
    else:
        raise AssertionError("agent regained root")
    try:
        fd = os.open("/usr/local/lib/bob/network.py", os.O_WRONLY)
    except PermissionError:
        pass
    else:
        os.close(fd)
        raise AssertionError("agent can rewrite privileged bootstrap")
for path in ("/var/run/docker.sock", "/run/podman/podman.sock"):
    assert not os.path.exists(path), "host-control socket exposed: " + path
`
	dropPrivileges := `exec setpriv --reuid="$BOB_UID" --regid="$BOB_GID" --clear-groups --bounding-set=-all --inh-caps=-all --ambient-caps=-all --no-new-privs python3 -c "$1"`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "sh", "-c", dropPrivileges, "agent-check", check).CombinedOutput(); err != nil {
		t.Fatalf("agent containment failed: %s", output)
	}
	output, err = exec.CommandContext(ctx, binary, "run", "--rm", "--network", "none", "--security-opt", "seccomp=unconfined", "--env", "BOB_EGRESS_URLS=[]", DefaultOptions().Image, "--version").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "seccomp filtering is required") {
		t.Fatalf("image did not reject unconfined startup: %s, %v", output, err)
	}
	for _, tool := range []string{"iptables", "ip6tables"} {
		output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, tool, "-S", "OUTPUT").CombinedOutput()
		if err != nil || !strings.Contains(string(output), "-P OUTPUT DROP") {
			t.Fatalf("missing default-deny policy: %s %v", output, err)
		}
		if output, err := exec.CommandContext(ctx, binary, "exec", "--user", "1000:1000", connection.Container, tool, "-P", "OUTPUT", "ACCEPT").CombinedOutput(); err == nil {
			t.Fatalf("unprivileged process could change firewall: %s", output)
		}
	}
	id := manager.name
	if _, err := manager.Start(ctx, Options{}); err == nil {
		t.Fatal("allowed another concurrent backend in the same app")
	}
	if err := manager.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(ctx); err != nil {
		t.Fatal("stop should be idempotent")
	}

	remaining, err := command(ctx, binary, nil, "ps", "--all", "--filter", "id="+id, "--format", "{{.Names}}")
	if err != nil || remaining != "" {
		t.Fatal("container remains after shutdown")
	}

	// Restart with two writable directories and one read-only directory.
	second := t.TempDir()
	for _, directory := range []string{workspace, second} {
		if err := os.Chmod(directory, 0777); err != nil {
			t.Fatal(err)
		}
	}
	connection, err = manager.Start(ctx, Options{
		Runtime:    "docker",
		HomeVolume: volume,
		Workdir:    workspace,
		ReadWrite:  []string{second, workspace},
		MCPServers: map[string]MCPServer{"bob": {URL: fmt.Sprintf("http://host.docker.internal:%d/mcp", listener.Addr().(*net.TCPAddr).Port)}},
		ReadOnly:   []string{reference},
		BaseURL:    baseURL,
		Model:      "demo-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.Workspace != workspace || connection.ReadOnly {
		t.Fatal("did not use the dedicated working directory")
	}
	check = `test "$PWD" = "$1" || exit 1; for directory in "$1" "$2"; do touch "$directory/probe" || exit 1; rm "$directory/probe" || exit 1; done; if touch "$3/probe" 2>/tmp/error; then exit 1; fi; grep -q 'Read-only file system' /tmp/error`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "sh", "-c", agentShell, "agent-check", check, "mount-check", workspace, second, reference).CombinedOutput(); err != nil {
		t.Fatalf("mixed mount permissions failed: %s", output)
	}

	check = `import urllib.request
import os, json
url = json.loads(os.environ["OPENCODE_CONFIG_CONTENT"])["mcp"]["bob"]["url"]
assert urllib.request.urlopen(url+"?session=synthetic").read() == b'{"synthetic":"mcp"}'
`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "python3", "-c", check).CombinedOutput(); err != nil {
		t.Fatalf("remote MCP connection failed: %s", output)
	}

	if err := manager.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	// Bootstrap failure must report its cause and leave no active container.
	_, err = manager.Start(ctx, Options{Runtime: "docker", HomeVolume: volume, Workdir: workspace, BaseURL: "https://missing.bob-test.invalid/agent/", Model: "demo-model"})
	if err == nil || !strings.Contains(err.Error(), "container logs:") || manager.name != "" {
		t.Fatalf("bootstrap did not fail closed and clean up: %v", err)
	}

}

func TestConnectionReportsNamedMCPPorts(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "engine")
	script := `#!/bin/sh
[ "$1" = port ] && [ "$2" = owned-container ] || exit 1
case "$3" in
  4096/tcp) echo 0.0.0.0:40000 ;;
  19000/tcp) echo 127.0.0.1:40001 ;;
  19001/tcp) echo 127.0.0.1:40002 ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	input := DefaultOptions()
	input.Name = "owned-container"
	input.MCPServers = map[string]MCPServer{
		"bec_mcp":     {Port: 19000},
		"bob":         {Port: 19001},
		"remote_only": {URL: "https://example.invalid/mcp"},
	}
	options := resolvedOptions{Options: input, binary: binary, mounts: []mount{{path: "/workspace"}}}
	connection, err := getConnection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if connection.URL != "http://127.0.0.1:40000" || len(connection.MCPPorts) != 2 || connection.MCPPorts["bec_mcp"] != "127.0.0.1:40001" || connection.MCPPorts["bob"] != "127.0.0.1:40002" {
		t.Fatal("published ports were not associated with their MCP names")
	}
	encoded, err := json.Marshal(connection)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"mcpPorts":{`) || strings.Contains(string(encoded), `"mcpPort":`) {
		t.Fatal("connection JSON does not use named MCP ports")
	}
	if len(connection.MCPServers) != 1 || connection.MCPServers["remote_only"].URL != "https://example.invalid/mcp" {
		t.Fatalf("servers without a URL got an endpoint: %+v", connection.MCPServers)
	}
	options.MCPServers = nil
	connection, err = getConnection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(connection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"mcpPorts"`) {
		t.Fatal("included MCP ports without published MCP servers")
	}
}

func TestCheckTellsAStoppedBobFromAnUnansweredEngine(t *testing.T) {
	engine := filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
test "$*" = "inspect --format {{.State.Running}} bob-id" || { echo "unexpected: $*" >&2; exit 2; }
case "$BOB_TEST_CONTAINER" in
  running) echo true ;;
  exited) echo false ;;
  gone) echo "Error: No such object: bob-id" >&2; exit 1 ;;
  *) echo "Cannot connect to the Docker daemon at unix:///var/run/docker.sock" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(engine, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{binary: engine, name: "bob-id"}
	for _, test := range []struct {
		container string
		stopped   bool
		ok        bool
	}{
		{"running", false, true},
		{"exited", true, false},
		{"gone", true, false},
		{"no-engine", false, false},
	} {
		t.Setenv("BOB_TEST_CONTAINER", test.container)
		err := manager.Check(context.Background())
		if (err == nil) != test.ok || errors.Is(err, ErrStopped) != test.stopped {
			t.Errorf("%s: Check() = %v", test.container, err)
		}
	}

	if err := (&Manager{}).Check(context.Background()); !errors.Is(err, ErrStopped) {
		t.Errorf("Check without a container = %v", err)
	}
	done := make(chan struct{})
	close(done)
	t.Setenv("BOB_TEST_CONTAINER", "running")
	manager.hostMCPs = []*hostMCPProcess{{name: "bec", done: done, err: errors.New("exit status 1")}}
	if err := manager.Check(context.Background()); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "host MCP bec exited") {
		t.Errorf("Check with an exited host MCP server = %v", err)
	}
}

func TestConnectionTellsWhereTheHostReachesEachMCPServer(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "engine")
	script := `#!/bin/sh
case "$3" in
  4096/tcp) echo 0.0.0.0:40000 ;;
  19000/tcp) echo 0.0.0.0:40001 ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	input := DefaultOptions()
	input.Name = "owned-container"
	input.MCPServers = map[string]MCPServer{
		"bec_mcp": {URL: "http://host.docker.internal:8000/mcp", Plugin: "bec"},
		"bec_dev": {URL: "http://host.containers.internal:8015/mcp", Plugin: "bec"},
		"inside":  {URL: "http://localhost:19000/mcp", Port: 19000},
		"remote":  {URL: "https://mcp.example/tools"},
	}
	options := resolvedOptions{Options: input, binary: binary, mounts: []mount{{path: "/workspace"}}}
	connection, err := getConnection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]MCPEndpoint{
		"bec_mcp": {URL: "http://127.0.0.1:8000/mcp", Plugin: "bec"},
		"bec_dev": {URL: "http://127.0.0.1:8015/mcp", Plugin: "bec"},
		"inside":  {URL: "http://127.0.0.1:40001/mcp"},
		"remote":  {URL: "https://mcp.example/tools"},
	}
	if !reflect.DeepEqual(connection.MCPServers, want) {
		t.Fatalf("mcpServers = %+v", connection.MCPServers)
	}
	encoded, err := json.Marshal(connection.MCPServers["inside"])
	if err != nil || string(encoded) != `{"url":"http://127.0.0.1:40001/mcp"}` {
		t.Fatalf("an endpoint without a plugin encodes as %s", encoded)
	}
}
