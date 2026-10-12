package containermanager

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLaunchPreservesIdentityEndpointsAndArguments(t *testing.T) {
	input := DefaultOptions()
	input.Runtime = "docker"
	input.BaseURL = "https://provider.example/agent/"
	input.MCPPort = 19000
	input.MCPURL = "http://mcp.example:8000/mcp"
	input.APIKey = "synthetic-token"
	options := resolvedOptions{Options: input, workdir: "/workspace/$project", password: "synthetic-password",
		userConfig: userSettings{User: "1234:5678", Groups: []string{"9012", "5678"}}, mounts: []mount{{path: "/workspace/$project"}, {path: "/references", readOnly: true}}}
	args, env, err := getLaunchArgs(options)
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(args, " ")
	for _, value := range []string{"--user 0:0", "--cap-drop ALL", "--cap-add NET_ADMIN", "/workspace/$project:/workspace/$project", "/references:/references:ro", "--publish 0.0.0.0::4096/tcp", "--publish 127.0.0.1::19000/tcp"} {
		if !strings.Contains(command, value) {
			t.Fatalf("missing launch setting %s", value)
		}
	}
	if strings.Contains(command, input.APIKey) {
		t.Fatal("token exposed in CLI arguments")
	}
	settings := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		settings[key] = value
	}
	if settings["BOB_UID"] != "1234" || settings["BOB_GID"] != "5678" || settings["BOB_GROUPS"] != "9012,5678" {
		t.Fatal("lost caller identity")
	}
	if settings["API_KEY"] != input.APIKey || settings["LLM_BASE_URL"] != input.BaseURL {
		t.Fatal("lost provider settings")
	}
	var endpoints []string
	if err := json.Unmarshal([]byte(settings["BOB_EGRESS_URLS"]), &endpoints); err != nil || len(endpoints) != 2 || endpoints[1] != input.MCPURL {
		t.Fatal("incorrect egress endpoints")
	}
	var config struct {
		MCP map[string]struct {
			URL string `json:"url"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(settings["OPENCODE_CONFIG_CONTENT"]), &config); err != nil || config.MCP["bob"].URL != input.MCPURL {
		t.Fatal("incorrect remote MCP URL")
	}
	if !strings.Contains(command, "serve --hostname 0.0.0.0 --port 4096") {
		t.Fatal("default command is not serve")
	}
	options.Web = true
	webArgs, _, err := getLaunchArgs(options)
	if err != nil || !strings.Contains(strings.Join(webArgs, " "), "web --hostname 0.0.0.0 --port 4096") {
		t.Fatal("web command was not selected")
	}
	options.MCPURL = ""
	options.MCPCommand = []string{"sh", "-c", `printf '%s' "$HOME"`}
	_, env, err = getLaunchArgs(options)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			var config struct {
				MCP map[string]struct {
					Command []string `json:"command"`
				} `json:"mcp"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(entry, "OPENCODE_CONFIG_CONTENT=")), &config); err != nil || config.MCP["bob"].Command[2] != options.MCPCommand[2] {
				t.Fatal("MCP arguments changed")
			}
		}
	}
}
