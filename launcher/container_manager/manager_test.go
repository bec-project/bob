package containermanager

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sandboxEngine puts stand-ins for bwrap, socat and ripgrep first on PATH, makes a stand-in OpenCode
// (which runs sandbox-runtime) and sandbox-runtime, and returns options that pass validation. The
// stand-in OpenCode records its arguments and environment in the returned folder (it gets no
// variable of ours to find it), then exits with status 3.
func sandboxEngine(t *testing.T) (Options, string) {
	t.Helper()
	previous := sandboxSupported
	sandboxSupported = true
	t.Cleanup(func() { sandboxSupported = previous })
	root := t.TempDir()
	openCode := filepath.Join(root, "opencode")
	script := `#!/bin/sh
printf '%s\n' "$*" > "` + root + `/args"
/usr/bin/env > "` + root + `/env"
echo 'fake srt: no sandbox here' >&2
exit 3
`
	if err := os.WriteFile(openCode, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"bwrap", "socat", "rg"} {
		if err := os.WriteFile(filepath.Join(root, tool), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	runtime := filepath.Join(root, "cli.js")
	if err := os.WriteFile(runtime, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	workdir := filepath.Join(root, "workspace")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Workdir = workdir
	options.OpenCode = openCode
	options.SandboxRuntime = runtime
	options.Home = filepath.Join(root, "home")
	options.BaseURL = "https://model.example/agent/"
	return options, root
}

func staticELF() []byte {
	header := make([]byte, 64)
	copy(header, "\x7fELF")
	header[4], header[5], header[6] = 2, 1, 1      // 64-bit, little-endian, version 1
	binary.LittleEndian.PutUint16(header[16:], 2)  // an executable
	binary.LittleEndian.PutUint16(header[18:], 62) // x86_64
	binary.LittleEndian.PutUint32(header[20:], 1)
	binary.LittleEndian.PutUint16(header[52:], 64) // header size
	binary.LittleEndian.PutUint16(header[54:], 56) // program header size
	binary.LittleEndian.PutUint16(header[58:], 64) // section header size
	return header
}

func TestStartReportsASandboxThatExitsAndCleansUp(t *testing.T) {
	options, root := sandboxEngine(t)
	options.APIKey = "synthetic-token"
	manager := &Manager{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := manager.Start(ctx, options)
	if err == nil || !strings.Contains(err.Error(), "OpenCode exited before becoming ready") || !strings.Contains(err.Error(), "fake srt") {
		t.Fatalf("Start = %v", err)
	}
	if manager.sandbox != nil {
		t.Fatal("a failed start kept its sandbox")
	}
	args, err := os.ReadFile(filepath.Join(root, "args"))
	if err != nil || strings.Contains(string(args), options.APIKey) {
		t.Fatalf("bwrap arguments %q (%v) carry the token or were not written", args, err)
	}
	env, err := os.ReadFile(filepath.Join(root, "env"))
	if err != nil || !strings.Contains(string(env), "API_KEY="+options.APIKey) || strings.Contains(string(env), "BOB_TEST_STATE=") {
		t.Fatalf("the sandbox's environment is not its own: %v", err)
	}
	if _, err := os.Stat(options.Home); err != nil {
		t.Fatalf("bob's home folder was not created: %v", err)
	}
}

func TestCheckFollowsTheSandboxAndItsHostMCPServers(t *testing.T) {
	if err := (&Manager{}).Check(context.Background()); !errors.Is(err, ErrStopped) {
		t.Errorf("Check without a sandbox = %v", err)
	}
	running := &sandboxProcess{done: make(chan struct{})}
	manager := &Manager{sandbox: running}
	if err := manager.Check(context.Background()); err != nil {
		t.Errorf("Check of a running sandbox = %v", err)
	}
	running.err = errors.New("exit status 1")
	close(running.done)
	if err := manager.Check(context.Background()); !errors.Is(err, ErrStopped) {
		t.Errorf("Check of an exited sandbox = %v", err)
	}
	done := make(chan struct{})
	close(done)
	manager = &Manager{sandbox: &sandboxProcess{done: make(chan struct{})}}
	manager.hostMCPs = []*hostMCPProcess{{name: "bec", done: done, err: errors.New("exit status 1")}}
	if err := manager.Check(context.Background()); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "host MCP bec exited") {
		t.Errorf("Check with an exited host MCP server = %v", err)
	}
}

func TestConnectionTellsWhereTheHostReachesEachMCPServer(t *testing.T) {
	for _, test := range []struct {
		server MCPServer
		want   string
	}{
		{MCPServer{URL: "http://host.docker.internal:8000/mcp"}, "http://127.0.0.1:8000/mcp"},
		{MCPServer{URL: "http://host.containers.internal:8015/mcp"}, "http://127.0.0.1:8015/mcp"},
		{MCPServer{URL: "https://mcp.example/tools"}, "https://mcp.example/tools"},
	} {
		if got := hostMCPURL(test.server); got != test.want {
			t.Errorf("%s is reached at %s, want %s", test.server.URL, got, test.want)
		}
	}
	encoded, err := json.Marshal(map[string]MCPEndpoint{"plain": {URL: "http://127.0.0.1:1/mcp"}, "bec": {URL: "http://127.0.0.1:2/mcp", Plugin: "bec"}})
	if err != nil || string(encoded) != `{"bec":{"url":"http://127.0.0.1:2/mcp","plugin":"bec"},"plain":{"url":"http://127.0.0.1:1/mcp"}}` {
		t.Fatalf("endpoints encode as %s", encoded)
	}
}

func TestOpenCodeEnvironment(t *testing.T) {
	const remote = `"mcp":{"bob":{"enabled":true,"oauth":false,"type":"remote","url":"http://mcp.example:8000/mcp"}}`
	// Only MCP tools: everything else of OpenCode's is denied.
	const deny, denyButBob = `"permission":{"*":"deny"}`, `"permission":{"*":"deny","bob_*":"allow"}`
	const limit = `"provider":{"self-hosted":{"models":{"Qwen3.8-27B":{"limit":{"context":262144,"output":32768}}}}}`
	for _, test := range []struct {
		name            string
		model           string
		context, output int
		mcp             bool
		// want is the exact OPENCODE_CONFIG_CONTENT value.
		want string
	}{
		{"unset", "", 0, 0, false, "{" + deny + "}"},
		{"unset with MCP", "", 0, 0, true, "{" + remote + "," + denyButBob + "}"},
		{"default output", "", 262144, 0, false, "{" + deny + "," + limit + "}"},
		{"default output with MCP", "", 262144, 0, true, "{" + remote + "," + denyButBob + "," + limit + "}"},
		{"small context", "", 65536, 0, false, "{" + deny + `,"provider":{"self-hosted":{"models":{"Qwen3.8-27B":{"limit":{"context":65536,"output":16384}}}}}}`},
		{"explicit output and model", "vendor/model:7b", 131072, 8192, true, "{" + remote + "," + denyButBob + `,"provider":{"self-hosted":{"models":{"vendor/model:7b":{"limit":{"context":131072,"output":8192}}}}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := DefaultOptions()
			input.BaseURL = "https://provider.example/v1"
			input.APIKey = "synthetic-token"
			if test.model != "" {
				input.Model = test.model
			}
			if test.mcp {
				input.MCPServers = map[string]MCPServer{"bob": {URL: "http://mcp.example:8000/mcp", HostCommand: []string{"/private/host/python"}}}
			}
			input.ContextLimit, input.OutputLimit = test.context, test.output
			environment, err := getOpenCodeEnvironment(resolvedOptions{Options: input, password: "synthetic-password"})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{
				"API_KEY=synthetic-token", "LLM_BASE_URL=https://provider.example/v1",
				"LLM_MODEL_ID=" + input.Model, "OPENCODE_SERVER_PASSWORD=synthetic-password",
				"OPENCODE_CONFIG_CONTENT=" + test.want,
			}
			if !reflect.DeepEqual(environment, want) {
				t.Fatalf("environment %v, want %v", environment, want)
			}
			if strings.Contains(strings.Join(environment, "\n"), "/private/host") {
				t.Fatal("a host MCP command reached OpenCode's configuration")
			}
		})
	}
}
