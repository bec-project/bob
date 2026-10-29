package sandbox

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
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
