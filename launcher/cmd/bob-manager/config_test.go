package main

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bob/sandbox"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bob.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOptionsLoadConfigAndResolvePaths(t *testing.T) {
	t.Setenv("API_KEY", "synthetic-key")
	// Configuration replaces the old environment overrides.
	t.Setenv("LLM_MODEL_ID", "ignored")
	path := writeConfig(t, `
[sandbox]
opencode = "/opt/bob/opencode"
home = "bob-home"

[provider]
base_url = "http://example.invalid/v1"
model = "custom-model"
context_limit = 262144
output_limit = 16_384

[mcp.bob]
host_command = ["python3", "/host/path with spaces/server.py", "--label", "a quoted \"value\""]
url = "http://127.0.0.1:19000/mcp"
`)
	input, err := getOptions([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	if input.OpenCode != "/opt/bob/opencode" || input.Home != filepath.Join(filepath.Dir(path), "bob-home") {
		t.Fatalf("lost sandbox configuration: %+v", input)
	}
	if !reflect.DeepEqual(input.MCPServers["bob"].HostCommand, []string{"python3", "/host/path with spaces/server.py", "--label", `a quoted "value"`}) {
		t.Fatal("MCP command arguments changed")
	}
	if input.BaseURL != "http://example.invalid/v1" || input.Model != "custom-model" || input.APIKey != "synthetic-key" || input.MCPServers["bob"].URL != "http://127.0.0.1:19000/mcp" {
		t.Fatal("lost provider or MCP configuration")
	}
	if input.ContextLimit != 262144 || input.OutputLimit != 16384 {
		t.Fatalf("lost model limits: context %d, output %d", input.ContextLimit, input.OutputLimit)
	}
}

func TestOptionsDefaults(t *testing.T) {
	path := writeConfig(t, `[provider]
base_url = "http://example.invalid/v1"`)
	t.Chdir(filepath.Dir(path))
	input, err := getOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	defaults := sandbox.DefaultOptions()
	if input.OpenCode != defaults.OpenCode || input.Home != defaults.Home || input.BindAddress != "127.0.0.1" || input.BaseURL != "http://example.invalid/v1" || input.Model != "Qwen3.8-27B" || len(input.MCPServers) != 0 || input.ContextLimit != 0 || input.OutputLimit != 0 {
		t.Fatal("incorrect defaults")
	}
}

func TestOptionsRejectInvalidConfig(t *testing.T) {
	for _, content := range []string{
		``,
		`[provider]
base_url = "http://example.invalid/v1"
paht = "/repo"`,
		`[provider]
api_key = "secret"`,
		`[provider]
base_url = 1`,
		`[provider]
base_url =`,
		`[sandbox]
image = "localhost/bob"
[provider]
base_url = "http://example.invalid/v1"`,
		`[extra]
key = "value"
[provider]
base_url = "http://example.invalid/v1"`,
		`{"provider": {"base_url": "http://example.invalid/v1"}}`,
	} {
		path := writeConfig(t, content)
		if _, err := getOptions([]string{"--config", path}); err == nil {
			t.Errorf("accepted invalid configuration: %s", content)
		}
	}
	if _, err := getOptions([]string{"--config", filepath.Join(t.TempDir(), "missing.toml")}); err == nil {
		t.Fatal("accepted a missing configuration file")
	}
}

func TestUnknownSettingsAreNamed(t *testing.T) {
	path := writeConfig(t, `[provider]
base_url = "http://example.invalid/v1"
[extra]
key = "value"`)
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "extra") {
		t.Fatalf("an unknown setting returned %v, want an error naming it", err)
	}
}

func TestOptionsRejectUnknownFlagsAndPositionalArguments(t *testing.T) {
	for _, args := range [][]string{{"--no-such-flag"}, {"--config"}, {"/repo"}} {
		if _, err := getOptions(args); err == nil {
			t.Errorf("accepted arguments: %v", args)
		}
	}
}

func TestOptionsHelp(t *testing.T) {
	if _, err := getOptions([]string{"--help"}); err != flag.ErrHelp {
		t.Fatalf("help returned %v", err)
	}
}

func TestOptionsMissingDefaultConfigShowsHelp(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := getOptions(nil); err != flag.ErrHelp {
		t.Fatalf("missing default config returned %v, want help", err)
	}
	if _, err := getOptions([]string{"--config", "bob.toml"}); !os.IsNotExist(err) {
		t.Fatalf("explicit missing config returned %v, want file error", err)
	}
	if err := os.WriteFile("bob.toml", []byte(`[provider]
base_url =`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := getOptions(nil); err == nil || err == flag.ErrHelp {
		t.Fatalf("invalid default config returned %v, want parsing error", err)
	}
}

func TestExampleConfig(t *testing.T) {
	_, err := loadConfig("../../../deploy/bob.example.toml")
	if err != nil && !strings.Contains(err.Error(), "provider.base_url is a placeholder") {
		t.Fatalf("example config could not be loaded: %v", err)
	}
}

func TestPlaceholderProviderURL(t *testing.T) {
	path := writeConfig(t, "[provider]\nbase_url = \"https://provider.example/v1\"")
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "provider.base_url is a placeholder") {
		t.Fatalf("placeholder URL needs a clear error: %v", err)
	}
}

func TestProviderTokenOverridesEnvironment(t *testing.T) {
	t.Setenv("API_KEY", "environment-token")
	for _, test := range []struct{ config, want string }{
		{"", "environment-token"},
		{`token = "config-token"`, "config-token"},
		{`token = ""`, ""},
	} {
		path := writeConfig(t, "[provider]\nbase_url = \"http://example.invalid/v1\"\n"+test.config)
		input, err := loadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if input.APIKey != test.want {
			t.Fatal("incorrect token precedence")
		}
	}
}

func TestOptionsRequireProviderURL(t *testing.T) {
	if sandbox.DefaultOptions().BaseURL != "" {
		t.Fatal("default options supply a provider URL")
	}
	path := writeConfig(t, "")
	if _, err := loadConfig(path); err == nil {
		t.Fatal("accepted a config without provider.base_url")
	}
}

func TestRemoteMCPAndBindAddress(t *testing.T) {
	path := writeConfig(t, `[sandbox]
bind_address = "0.0.0.0"
[provider]
base_url = "http://example.invalid/agent/"
[mcp.bob]
url = "http://mcp.example:8000/mcp"
`)
	input, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if input.MCPServers["bob"].URL != "http://mcp.example:8000/mcp" || input.BindAddress != "0.0.0.0" {
		t.Fatal("remote MCP or API binding lost")
	}
}

func TestNamedMCPConfig(t *testing.T) {
	path := writeConfig(t, `[provider]
base_url = "http://example.invalid/v1"
[mcp.bec_mcp]
url = "http://mcp.example:8000/mcp"
plugin = "bec"
[mcp.other_tools]
url = "https://tools.example/mcp"
`)
	input, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]sandbox.MCPServer{
		"bec_mcp":     {URL: "http://mcp.example:8000/mcp", Plugin: "bec"},
		"other_tools": {URL: "https://tools.example/mcp"},
	}
	if !reflect.DeepEqual(input.MCPServers, want) {
		t.Fatal("named MCP configuration was not preserved")
	}
}

func TestMCPConfigRejectsUnknownFieldsAndWrongTypes(t *testing.T) {
	for _, content := range []string{
		"[mcp]\nprot = 19000",
		"[mcp]\nurl = 123",
		"[mcp.bec_mcp]\nurll = \"http://mcp.example/mcp\"",
		"[mcp]\nurl = \"http://mcp.example/mcp\"",
		"[mcp.bec_mcp]\nurl = \"http://mcp.example/mcp\"\nplugin = 1",
	} {
		path := writeConfig(t, "[provider]\nbase_url = \"http://example.invalid/v1\"\n"+content)
		if _, err := loadConfig(path); err == nil {
			t.Errorf("accepted invalid MCP config: %s", content)
		}
	}
}

func TestHostMCPConfigNeedsNoWorkingDirectory(t *testing.T) {
	path := writeConfig(t, `[provider]
base_url = "http://example.invalid/v1"
[mcp.bec_mcp]
url = "http://127.0.0.1:19000/mcp"
host_command = ["/host/python", "-m", "bec_mcp_server", "--allow-unauthenticated"]
`)
	input, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	server := input.MCPServers["bec_mcp"]
	if len(server.HostCommand) != 4 {
		t.Fatal("host MCP configuration was not preserved")
	}
}

func TestProviderModelLimitConfig(t *testing.T) {
	for _, test := range []struct {
		config                  string
		wantContext, wantOutput int
	}{
		{"", 0, 0},
		{"context_limit = 262144", 262144, 0},
		{"context_limit = 262144\noutput_limit = 32768", 262144, 32768},
		{"context_limit = 0\noutput_limit = 0", 0, 0},
		// Range checks belong to manager validation, which reports the setting by name.
		{"context_limit = -1\noutput_limit = 32768", -1, 32768},
	} {
		path := writeConfig(t, "[provider]\nbase_url = \"http://example.invalid/v1\"\n"+test.config)
		input, err := loadConfig(path)
		if err != nil {
			t.Fatalf("%q: %v", test.config, err)
		}
		if input.ContextLimit != test.wantContext || input.OutputLimit != test.wantOutput {
			t.Errorf("%q loaded context %d, output %d", test.config, input.ContextLimit, input.OutputLimit)
		}
	}
}

func TestProviderModelLimitConfigRejectsWrongTypes(t *testing.T) {
	for _, content := range []string{
		`context_limit = "262144"`,
		`context_limit = 262144.5`,
		`context_limit = [262144]`,
		`output_limit = true`,
		`context_limit = 99999999999999999999`,
		`contextlimit = 262144`,
		`max_tokens = 32768`,
	} {
		path := writeConfig(t, "[provider]\nbase_url = \"http://example.invalid/v1\"\n"+content)
		if _, err := loadConfig(path); err == nil {
			t.Errorf("accepted invalid model limit config: %s", content)
		}
	}
}
