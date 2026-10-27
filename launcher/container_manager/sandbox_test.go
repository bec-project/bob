package containermanager

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSandboxEndpointsKeepNamesAndMoveLowPorts(t *testing.T) {
	endpoints, inside, hosts, err := getSandboxEndpoints([]string{
		"https://ai.example/api",
		"http://host.containers.internal:8020/mcp",
		"http://192.0.2.7:9000/mcp",
		"https://ai.example:8443/other",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []sandboxEndpoint{
		{Listen: "127.0.0.2:10443", Socket: "out-0.sock", target: "ai.example:443"},
		{Listen: "127.0.0.3:8020", Socket: "out-1.sock", target: "127.0.0.1:8020"},
		{Listen: "127.0.0.4:9000", Socket: "out-2.sock", target: "192.0.2.7:9000"},
		{Listen: "127.0.0.2:8443", Socket: "out-3.sock", target: "ai.example:8443"},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Fatalf("endpoints %+v", endpoints)
	}
	// Names stay, so TLS checks the real certificate; an address has no name to keep.
	for raw, expected := range map[string]string{
		"https://ai.example/api":                   "https://ai.example:10443/api",
		"http://host.containers.internal:8020/mcp": "http://host.containers.internal:8020/mcp",
		"http://192.0.2.7:9000/mcp":                "http://127.0.0.4:9000/mcp",
	} {
		if inside[raw] != expected {
			t.Fatalf("%s reached as %s", raw, inside[raw])
		}
	}
	if !reflect.DeepEqual(hosts, []string{"127.0.0.2 ai.example", "127.0.0.3 host.containers.internal"}) {
		t.Fatalf("hosts %v", hosts)
	}
}

func TestSandboxEndpointsRefuseLoopbackAndCollisions(t *testing.T) {
	for _, urls := range [][]string{
		{"http://localhost:8000/mcp"},
		{"http://127.0.0.1:8000/mcp"},
		{"https://ai.example/", "https://ai.example:10443/"},
	} {
		if _, _, _, err := getSandboxEndpoints(urls); err == nil {
			t.Fatalf("accepted %v", urls)
		}
	}
}

func sandboxTestOptions(t *testing.T) resolvedOptions {
	input := DefaultOptions()
	input.Runtime = bubblewrapRuntime
	input.BaseURL = "https://ai.example/api"
	input.APIKey = "synthetic-token"
	input.Home = "/data/bob/home"
	input.MCPServers = map[string]MCPServer{"bec": {URL: "http://host.containers.internal:8020/mcp", Plugin: "bec"}}
	return resolvedOptions{Options: input, workdir: "/beamline/work", password: "synthetic-password",
		mounts: []mount{{path: "/beamline/work"}, {path: "/beamline/data", readOnly: true}}}
}

func TestSandboxSeesOnlyWhatItIsGiven(t *testing.T) {
	options := sandboxTestOptions(t)
	args := getSandboxArgs(options, "/run/user/1000/bob-1", "/usr/bin/bob-manager", "/opt/bob/opencode")
	command := strings.Join(args, " ")
	for _, value := range []string{
		"--unshare-all", "--die-with-parent", "--new-session",
		"--ro-bind /usr /usr", "--ro-bind /etc /etc", "--ro-bind /run/user/1000/bob-1/hosts /etc/hosts",
		"--bind /run/user/1000/bob-1 /run/bob", "--bind /data/bob/home /home/opencode",
		"--ro-bind /usr/bin/bob-manager /bob/bin/bob-manager", "--ro-bind /opt/bob/opencode /bob/bin/opencode",
		"--bind /beamline/work /beamline/work", "--ro-bind /beamline/data /beamline/data",
		"--chdir /beamline/work /bob/bin/bob-manager --sandbox-helper",
	} {
		if !strings.Contains(command, value) {
			t.Fatalf("missing %q in %s", value, command)
		}
	}
	// Neither the host's root nor the user's home comes in.
	for i := 0; i+2 < len(args); i++ {
		if strings.HasSuffix(args[i], "bind") && (args[i+1] == "/" || args[i+1] == os.Getenv("HOME")) {
			t.Fatalf("binds %s", args[i+1])
		}
	}
	if strings.Contains(command, options.APIKey) || strings.Contains(command, options.password) {
		t.Fatal("secret in the command line")
	}
}

func TestSandboxEnvironmentPointsOpenCodeAtItsTunnels(t *testing.T) {
	options := sandboxTestOptions(t)
	endpoints, inside, _, err := getSandboxEndpoints([]string{options.BaseURL, options.MCPServers["bec"].URL})
	if err != nil {
		t.Fatal(err)
	}
	env, err := getSandboxEnvironment(options, inside, endpoints)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		settings[key] = value
	}
	if settings["LLM_BASE_URL"] != "https://ai.example:10443/api" || settings["API_KEY"] != options.APIKey {
		t.Fatalf("provider settings %q", settings["LLM_BASE_URL"])
	}
	if settings["HOME"] != sandboxHome || settings["OPENCODE_CONFIG"] != "/run/bob/opencode.json" {
		t.Fatal("OpenCode's home or configuration is not the sandbox's")
	}
	if _, found := settings["BOB_EGRESS_URLS"]; found {
		t.Fatal("container settings leak into the sandbox")
	}
	var plan sandboxPlan
	if err := json.Unmarshal([]byte(settings[sandboxPlanVariable]), &plan); err != nil || len(plan.Endpoints) != 2 || plan.Server != sandboxServer {
		t.Fatalf("plan %+v: %v", plan, err)
	}
	if !strings.Contains(settings["OPENCODE_CONFIG_CONTENT"], `"url":"http://host.containers.internal:8020/mcp"`) {
		t.Fatalf("MCP configuration %s", settings["OPENCODE_CONFIG_CONTENT"])
	}
}

func TestOpenCodeConfigMatchesTheImage(t *testing.T) {
	image, err := os.ReadFile(filepath.Join("..", "..", "containers", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(image, openCodeConfig) {
		t.Fatal("container_manager/opencode.json differs from containers/opencode.json; copy it over")
	}
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
