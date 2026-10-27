package containermanager

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func sandboxTestOptions(t *testing.T) resolvedOptions {
	input := DefaultOptions()
	input.BaseURL = "https://ai.example/api"
	input.APIKey = "synthetic-token"
	input.Home = "/data/bob/home"
	input.MCPServers = map[string]MCPServer{"bec": {URL: "http://host.containers.internal:8020/mcp", Plugin: "bec"}}
	return resolvedOptions{Options: input, workdir: "/beamline/work", password: "synthetic-password",
		mounts: []mount{{path: "/beamline/work"}, {path: "/beamline/data", readOnly: true}}}
}

func TestTunnelsCarryAConnectionBothWays(t *testing.T) {
	// Short, since socket paths are limited to about 100 bytes.
	directory, err := os.MkdirTemp("/tmp", "bob-tunnel-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		conn, err := echo.Accept()
		if err != nil {
			return
		}
		data := make([]byte, 5)
		if _, err := conn.Read(data); err == nil {
			conn.Write(bytes.ToUpper(data))
		}
		conn.Close()
	}()
	// Client to TCP to Unix socket to TCP, as clients reach OpenCode and OpenCode its endpoints.
	socket, err := net.Listen("unix", filepath.Join(directory, "out.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	go serveTunnel(socket, dialTCP(echo.Addr().String()))
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer front.Close()
	go serveTunnel(front, dialUnix(filepath.Join(directory, "out.sock")))

	conn, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("hello"))
	conn.(*net.TCPConn).CloseWrite()
	reply := make([]byte, 16)
	n, _ := conn.Read(reply)
	if string(reply[:n]) != "HELLO" {
		t.Fatalf("reply %q", reply[:n])
	}
}

func TestOpenCodeIsFoundBesideBobManager(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"bin", "libexec", "path"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	helper := filepath.Join(root, "bin", "bob-manager")
	bundled := filepath.Join(root, "libexec", "opencode")
	onPath := filepath.Join(root, "path", "opencode")
	for _, executable := range []string{helper, onPath} {
		if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Join(root, "path"))
	if got, err := findOpenCode("", helper); err != nil || got != onPath {
		t.Fatalf("without a bundled OpenCode: %q, %v", got, err)
	}
	if err := os.WriteFile(bundled, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := findOpenCode("", helper); err != nil || got != bundled {
		t.Fatalf("with a bundled OpenCode: %q, %v", got, err)
	}
	if got, err := findOpenCode(onPath, helper); err != nil || got != onPath {
		t.Fatalf("sandbox.opencode did not win: %q, %v", got, err)
	}
	if _, err := findOpenCode("missing-opencode", helper); err == nil {
		t.Fatal("found a missing OpenCode")
	}
}

func TestSRTEndpointsAllowEachHostAndPort(t *testing.T) {
	allowed, inside, err := getSRTEndpoints([]string{
		"https://ai.example/api",
		"http://host.containers.internal:8020/mcp",
		"http://192.0.2.7:9000/mcp",
		"https://ai.example/api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowed, []string{"ai.example:443", "127.0.0.1:8020", "192.0.2.7:9000"}) {
		t.Fatalf("allowed %v", allowed)
	}
	// srt's proxy dials this machine as an allow-listed literal; other URLs stay as they are.
	if inside["http://host.containers.internal:8020/mcp"] != "http://127.0.0.1:8020/mcp" || inside["https://ai.example/api"] != "https://ai.example/api" {
		t.Fatalf("inside %v", inside)
	}
	for _, raw := range []string{"http://localhost:8000/mcp", "http://127.0.0.1:8000/mcp"} {
		if _, _, err := getSRTEndpoints([]string{raw}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestSRTEnvironmentRunsSRTOnBunAndKeepsItFromOpenCode(t *testing.T) {
	options := sandboxTestOptions(t)
	_, inside, err := getSRTEndpoints([]string{options.BaseURL, options.MCPServers["bec"].URL})
	if err != nil {
		t.Fatal(err)
	}
	tools := sandboxTools{bash: "/usr/bin/bash", socat: "/usr/bin/socat", bwrap: "/usr/bin/bwrap"}
	env, err := getSRTEnvironment(options, inside, "/opt/bob/opencode", "/run/bob-1/sockets", "/run/bob-1/opencode.json", tools, "/run/bob-1/seccomp.bpf")
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		settings[key] = value
	}
	if settings["BUN_BE_BUN"] != "1" || settings[shimRealVariable] != "/usr/bin/bwrap" || settings[shimSeccompVariable] != "/run/bob-1/seccomp.bpf" {
		t.Fatalf("srt runner or shim settings missing: %v", settings)
	}
	if settings["HOME"] != options.Home || settings["LLM_BASE_URL"] != options.BaseURL {
		t.Fatal("OpenCode's home or provider URL changed")
	}
	if !strings.Contains(settings["OPENCODE_CONFIG_CONTENT"], `"url":"http://127.0.0.1:8020/mcp"`) {
		t.Fatalf("MCP configuration %s", settings["OPENCODE_CONFIG_CONTENT"])
	}
	var plan sandboxPlan
	if err := json.Unmarshal([]byte(settings[sandboxPlanVariable]), &plan); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"BUN_BE_BUN", shimRealVariable, shimSeccompVariable, "NO_PROXY", "no_proxy"} {
		if !slices.Contains(plan.Unset, name) {
			t.Errorf("OpenCode would get %s", name)
		}
	}
	if plan.Sockets != "/run/bob-1/sockets" || len(plan.Endpoints) != 0 {
		t.Fatalf("plan %+v", plan)
	}
}
