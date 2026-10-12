package containermanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	if err := os.Chmod(workspace, 0777); err != nil {
		t.Fatal(err)
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
		MCPPort:    19000,
		APIKey:     "synthetic-provider-token",
		MCPCommand: []string{"python3", "-u", "-c", `
import json, sys
for line in sys.stdin:
    request = json.loads(line)
    if "id" not in request:
        continue
    if request["method"] == "initialize":
        result = {"protocolVersion": request["params"]["protocolVersion"], "capabilities": {"tools": {}}, "serverInfo": {"name": "bob-test", "version": "1"}}
    elif request["method"] == "tools/list":
        result = {"tools": []}
    else:
        result = {}
    print(json.dumps({"jsonrpc": "2.0", "id": request["id"], "result": result}), flush=True)
`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.MCPPort == "" || connection.Password == "" {
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
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "sh", "-c", check, "mount-check", workspace, reference).CombinedOutput(); err != nil {
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
			Type    string   `json:"type"`
			Command []string `json:"command"`
			Enabled bool     `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	if config.Model != "self-hosted/demo-model" || len(config.Enabled) != 1 || config.Enabled[0] != "self-hosted" {
		t.Fatal("backend did not load the restricted provider configuration")
	}
	if config.MCP["bob"].Type != "local" || !config.MCP["bob"].Enabled || len(config.MCP["bob"].Command) != 4 {
		t.Fatal("MCP launch command was not configured")
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
`
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "python3", "-c", check).CombinedOutput(); err != nil {
		t.Fatalf("agent retained bootstrap privileges: %s", output)
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
		MCPURL:     fmt.Sprintf("http://host.docker.internal:%d/mcp", listener.Addr().(*net.TCPAddr).Port),
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
	if output, err := exec.CommandContext(ctx, binary, "exec", connection.Container, "sh", "-c", check, "mount-check", workspace, second, reference).CombinedOutput(); err != nil {
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
