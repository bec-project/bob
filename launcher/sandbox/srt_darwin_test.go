package sandbox

import (
	"bytes"
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"testing"
)

func TestSRTEndpointsReachThisMachineOnLoopback(t *testing.T) {
	allowed, inside, err := getSRTEndpoints([]string{
		"https://ai.example/api",
		"http://localhost:8020/mcp",
		"http://127.0.0.1:8021/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowed, []string{"ai.example:443", "127.0.0.1:8020", "127.0.0.1:8021"}) {
		t.Fatalf("allowed %v", allowed)
	}
	// srt's NO_PROXY keeps loopback away from its proxy; allowLocalBinding lets OpenCode connect.
	if inside["http://localhost:8020/mcp"] != "http://127.0.0.1:8020/mcp" || inside["http://127.0.0.1:8021/mcp"] != "http://127.0.0.1:8021/mcp" {
		t.Fatalf("inside %v", inside)
	}
	for _, raw := range []string{"http://127.0.0.2:8000/mcp", "http://[::1]:8000/mcp"} {
		if _, _, err := getSRTEndpoints([]string{raw}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestSRTSettingsLetOpenCodeServeOnLoopback(t *testing.T) {
	var settings srtSettings
	applyPlatformSettings(&settings, sandboxTools{})
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Network.AllowLocalBinding {
		t.Fatal("OpenCode could not serve on loopback")
	}
	for _, key := range []string{"bwrapPath", "socatPath", "seccomp"} {
		if strings.Contains(string(encoded), `"`+key+`"`) {
			t.Errorf("macOS settings carry %s: %s", key, encoded)
		}
	}
}

func TestSRTEnvironmentNeedsNoSocat(t *testing.T) {
	options := sandboxTestOptions(t)
	_, inside, err := getSRTEndpoints([]string{options.BaseURL, options.MCPServers["bec"].URL})
	if err != nil {
		t.Fatal(err)
	}
	env, err := getSRTEnvironment(options, inside, openCodePolicy{directory: "/data/bob/home/.opencode-policy"}, sandboxTools{bash: "/bin/bash"}, "/data/bob/home/.cache/tmp")
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		settings[key] = value
	}
	if settings["PATH"] != "/bin:/usr/bin:/bin" {
		t.Errorf("PATH %q", settings["PATH"])
	}
	if !strings.Contains(settings["OPENCODE_CONFIG_CONTENT"], `"url":"http://127.0.0.1:8020/mcp"`) {
		t.Fatalf("MCP configuration %s", settings["OPENCODE_CONFIG_CONTENT"])
	}
}

func TestWayInForwardsToOpenCodesLoopbackPort(t *testing.T) {
	stdin, listener, port, err := openWayIn(t.TempDir(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if stdin != nil {
		t.Fatal("the helper got a socket on macOS, where OpenCode serves the way in itself")
	}
	echo, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
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
	conn, err := net.Dial("tcp", listener.Addr().String())
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

func TestConfinementProbeFailsOutsideTheSandbox(t *testing.T) {
	if err := verifySandboxConfinement(); err == nil {
		t.Fatal("the confinement probe passed without srt's sandbox")
	}
}
