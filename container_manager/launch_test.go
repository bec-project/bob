package containermanager

import (
	"encoding/json"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestLaunchPreservesIdentityEndpointsAndArguments(t *testing.T) {
	input := DefaultOptions()
	input.Runtime = "docker"
	input.BaseURL = "https://provider.example/agent/"
	input.MCPServers = map[string]MCPServer{"bob": {Port: 19000, URL: "http://mcp.example:8000/mcp"}}
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
	if err := json.Unmarshal([]byte(settings["BOB_EGRESS_URLS"]), &endpoints); err != nil || len(endpoints) != 2 || endpoints[1] != input.MCPServers["bob"].URL {
		t.Fatal("incorrect egress endpoints")
	}
	var config struct {
		MCP map[string]struct {
			URL string `json:"url"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(settings["OPENCODE_CONFIG_CONTENT"]), &config); err != nil || config.MCP["bob"].URL != input.MCPServers["bob"].URL {
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
}

func TestLaunchEnforcesEngineSeccompPolicy(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			input := DefaultOptions()
			input.Runtime = engine
			options := resolvedOptions{Options: input}
			args, _, err := getLaunchArgs(options)
			if err != nil {
				t.Fatal(err)
			}
			want := "--security-opt seccomp=builtin"
			if engine == "podman" {
				want = "--seccomp-policy default"
			}
			for _, command := range [][]string{args, getHomeOwnershipArgs(options, "1000:1000")} {
				if !strings.Contains(strings.Join(command, " "), want) {
					t.Fatalf("missing explicit seccomp policy: %v", command)
				}
			}
		})
	}
}

func TestMountLabelPolicyPreservesReadOnlyFlags(t *testing.T) {
	for _, policy := range []string{"", "shared", "private"} {
		t.Run(policy, func(t *testing.T) {
			input := DefaultOptions()
			input.MountLabel = policy
			options := resolvedOptions{Options: input, mounts: []mount{{path: "/workspace"}, {path: "/references", readOnly: true}}}
			args, _, err := getLaunchArgs(options)
			if err != nil {
				t.Fatal(err)
			}
			label := map[string]string{"": "", "shared": "z", "private": "Z"}[policy]
			writable, readOnly := "/workspace:/workspace", "/references:/references:ro"
			if label != "" {
				writable += ":" + label
				readOnly += "," + label
			}
			command := strings.Join(args, " ")
			if !strings.Contains(command, "--volume "+writable+" ") || !strings.Contains(command, "--volume "+readOnly+" ") {
				t.Fatalf("incorrect mount policy: %v", args)
			}
		})
	}
}

func TestDockerRejectsUnsupportedIdentityMappings(t *testing.T) {
	for _, info := range []string{`["name=rootless"]`, `["name=userns"]`, `["name=rootless,version=1"]`, "invalid"} {
		if err := validateDockerMapping(info); err == nil {
			t.Fatalf("accepted unsupported Docker mapping: %s", info)
		}
	}
	if err := validateDockerMapping(`["name=seccomp,profile=builtin","name=apparmor"]`); err != nil {
		t.Fatal(err)
	}
}

func TestHomeOwnershipIncludesExistingStateWithoutFollowingSymlinks(t *testing.T) {
	options := resolvedOptions{Options: DefaultOptions(), launchID: "owned"}
	options.Runtime = "docker"
	args := strings.Join(getHomeOwnershipArgs(options, "1234:5678"), " ")
	for _, want := range []string{"create", "--label " + launchLabel + "=owned-home", "-R -h 1234:5678 /home/opencode"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing home ownership policy %q: %s", want, args)
		}
	}
	launchArgs, _, err := getLaunchArgs(options)
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(launchArgs, " ")
	if runtime.GOOS == "linux" {
		if !strings.Contains(command, "--mount type=volume,src="+options.HomeVolume+",dst=/home/opencode,volume-nocopy") {
			t.Fatalf("Docker launch can overwrite prepared empty-home ownership: %v", launchArgs)
		}
	} else {
		if !strings.Contains(command, "--volume "+options.HomeVolume+":/home/opencode") || strings.Contains(command, "volume-nocopy") {
			t.Fatalf("Docker launch prevents image home ownership initialization: %v", launchArgs)
		}
	}
}

func TestLaunchNamedMCPServers(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			input := DefaultOptions()
			input.Runtime = engine
			input.BaseURL = "http://provider.example/v1"
			input.MCPServers = map[string]MCPServer{
				"bec_mcp":        {Port: 19000, URL: "http://bec.example:8000/mcp"},
				"another_remote": {Port: 19001, URL: "https://tools.example/mcp"},
			}
			args, env, err := getLaunchArgs(resolvedOptions{Options: input})
			if err != nil {
				t.Fatal(err)
			}
			for _, port := range []string{"127.0.0.1::19000/tcp", "127.0.0.1::19001/tcp"} {
				if !strings.Contains(strings.Join(args, " "), "--publish "+port) {
					t.Fatalf("missing named MCP port %s", port)
				}
			}
			settings := map[string]string{}
			for _, entry := range env {
				key, value, _ := strings.Cut(entry, "=")
				settings[key] = value
			}
			var endpoints []string
			if err := json.Unmarshal([]byte(settings["BOB_EGRESS_URLS"]), &endpoints); err != nil {
				t.Fatal(err)
			}
			wantEndpoints := []string{input.BaseURL, input.MCPServers["another_remote"].URL, input.MCPServers["bec_mcp"].URL}
			if !reflect.DeepEqual(endpoints, wantEndpoints) {
				t.Fatalf("egress endpoints: %v, want %v", endpoints, wantEndpoints)
			}
			var config struct {
				MCP map[string]struct {
					Type    string `json:"type"`
					URL     string `json:"url"`
					Enabled bool   `json:"enabled"`
					OAuth   *bool  `json:"oauth"`
				} `json:"mcp"`
			}
			if err := json.Unmarshal([]byte(settings["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
				t.Fatal(err)
			}
			if len(config.MCP) != len(input.MCPServers) {
				t.Fatal("lost named servers or added a hardcoded name")
			}
			for name, server := range input.MCPServers {
				got := config.MCP[name]
				if !got.Enabled || got.URL != server.URL {
					t.Fatalf("lost settings for %s", name)
				}
				if server.URL != "" && (got.Type != "remote" || got.OAuth == nil || *got.OAuth) {
					t.Fatalf("incorrect remote settings for %s", name)
				}

			}
		})
	}
}

func TestPortOnlyMCPDoesNotConfigureOpenCode(t *testing.T) {
	input := DefaultOptions()
	input.MCPServers = map[string]MCPServer{"bec_mcp": {Port: 19000}}
	args, env, err := getLaunchArgs(resolvedOptions{Options: input})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--publish 127.0.0.1::19000/tcp") {
		t.Fatal("port-only MCP entry was not published")
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			t.Fatal("port-only MCP entry added an OpenCode connection")
		}
	}
}

func TestHostMCPCommandIsNeverSentToContainer(t *testing.T) {
	options := resolvedOptions{Options: DefaultOptions()}
	options.BaseURL = "https://provider.example/v1"
	options.MCPServers = map[string]MCPServer{"bec_mcp": {
		URL:         "http://host.docker.internal:19000/mcp",
		HostCommand: []string{"/private/host/python", "-m", "private_host_module"},
	}}
	args, env, err := getLaunchArgs(options)
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.Join(append(args, env...), "\n")
	for _, forbidden := range []string{"/private/host", "private_host_module"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatal("host MCP command leaked into container configuration")
		}
	}
	if !strings.Contains(serialized, "host.docker.internal:19000/mcp") || strings.Contains(serialized, "6379") {
		t.Fatal("container did not retain just the MCP endpoint")
	}
}
