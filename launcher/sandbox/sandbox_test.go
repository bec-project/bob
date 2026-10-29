package sandbox

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sandboxTestOptions(t *testing.T) resolvedOptions {
	input := DefaultOptions()
	input.BaseURL = "https://ai.example/api"
	input.APIKey = "synthetic-token"
	input.Home = "/data/bob/home"
	input.MCPServers = map[string]MCPServer{"bec": {URL: "http://localhost:8020/mcp", Plugin: "bec"}}
	return resolvedOptions{Options: input, password: "synthetic-password"}
}

func TestWayInIsServedFromAnInheritedUnixListener(t *testing.T) {
	// Short, since socket paths are limited to about 100 bytes.
	directory, err := os.MkdirTemp("/tmp", "bob-wayin-")
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
	// The Unix socket bob-manager hands on as a file, the way the helper gets it as standard
	// input, and the TCP listener bob-manager forwards to it.
	path := filepath.Join(directory, sandboxServerSocket)
	inner, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	inner.(*net.UnixListener).SetUnlinkOnClose(false)
	file, err := inner.(*net.UnixListener).File()
	inner.Close()
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := net.FileListener(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer inherited.Close()
	go serveTunnel(inherited, dialTCP(echo.Addr().String()))
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer front.Close()
	go serveTunnel(front, dialUnix(path))

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

// stubHostLoopbackName makes hostLoopbackName resolve as RHEL's /etc/hosts has it.
func stubHostLoopbackName(t *testing.T) {
	previous := lookupHost
	lookupHost = func(host string) ([]string, error) {
		if host == hostLoopbackName {
			return []string{"127.0.0.1"}, nil
		}
		return previous(host)
	}
	t.Cleanup(func() { lookupHost = previous })
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
