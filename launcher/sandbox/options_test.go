package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBindAddressDefaultsToLoopback(t *testing.T) {
	if got := DefaultOptions().BindAddress; got != "127.0.0.1" {
		t.Fatalf("default bind address = %q, want loopback", got)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bwrap"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	for _, test := range []struct{ address, want string }{
		{"", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"0.0.0.0", "0.0.0.0"},
		{"::1", "::1"},
	} {
		options := DefaultOptions()
		options.BaseURL = "https://example.invalid/v1"
		options.Home = filepath.Join(root, "home")
		options.BindAddress = test.address
		resolved, err := getResolvedOptions(options)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.BindAddress != test.want {
			t.Errorf("bind address %q resolved to %q, want %q", test.address, resolved.BindAddress, test.want)
		}
	}
}

func TestEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://example.com", "https://user:secret@example.com", "https://example.com:70000", "https://example.com:0", "https://example.com/#fragment"} {
		if _, err := getEndpoint(endpoint); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
}

func TestNamedMCPValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		server MCPServer
	}{
		{"", MCPServer{URL: "http://example.invalid/mcp"}},
		{"  ", MCPServer{URL: "http://example.invalid/mcp"}},
		{"bad\x00name", MCPServer{URL: "http://example.invalid/mcp"}},
		{"bec_mcp", MCPServer{}},
		{"bec_mcp", MCPServer{URL: "ftp://example.invalid/mcp"}},
		{"bec_mcp", MCPServer{URL: "http://example.invalid/mcp", Plugin: "Bec"}},
		{"bec_mcp", MCPServer{URL: "http://example.invalid/mcp", Plugin: "../bec"}},
		{"bec_mcp", MCPServer{Plugin: "bec"}},
	} {
		input := DefaultOptions()
		input.BaseURL = "http://example.invalid/v1"
		input.MCPServers = map[string]MCPServer{test.name: test.server}
		if err := input.validate(); err == nil {
			t.Errorf("accepted invalid MCP server %q", test.name)
		}
	}
	input := DefaultOptions()
	input.BaseURL = "http://example.invalid/v1"
	input.MCPServers = map[string]MCPServer{"bob": {URL: "http://example.invalid/mcp"}}
	if err := input.validate(); err != nil {
		t.Fatal(err)
	}
	input.MCPServers = map[string]MCPServer{"bec_mcp": {URL: "http://example.invalid/mcp", Plugin: "bec"}}
	if err := input.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHostMCPValidation(t *testing.T) {
	for _, server := range []MCPServer{
		{HostCommand: []string{"python3"}},
		{URL: "http://127.0.0.1:19000/mcp", HostCommand: []string{" "}},
		{URL: "http://127.0.0.1:19000/mcp", HostCommand: []string{"python3", "bad\x00arg"}},
		{URL: "http://example.invalid/mcp", HostCommand: []string{"python3"}},
		{URL: "http://127.0.0.2:19000/mcp", HostCommand: []string{"python3"}},
		{URL: "http://[::1]:19000/mcp", HostCommand: []string{"python3"}},
	} {
		if err := server.validate(); err == nil {
			t.Fatal("accepted invalid host MCP configuration")
		}
	}
	for _, url := range []string{"http://127.0.0.1:19000/mcp", "http://localhost:19000/mcp"} {
		if err := (MCPServer{URL: url, HostCommand: []string{"python3"}}).validate(); err != nil {
			t.Fatalf("%s: %v", url, err)
		}
	}
}

func TestModelLimitValidation(t *testing.T) {
	for _, limits := range [][2]int{{0, 0}, {262144, 0}, {262144, 32768}, {262144, 262143}, {2, 1}} {
		input := DefaultOptions()
		input.BaseURL = "http://example.invalid/v1"
		input.ContextLimit, input.OutputLimit = limits[0], limits[1]
		if err := input.validate(); err != nil {
			t.Errorf("rejected valid limits %v: %v", limits, err)
		}
	}
	for _, test := range []struct {
		limits [2]int
		want   string
	}{
		{[2]int{-1, 0}, "provider.context_limit must not be negative"},
		{[2]int{-262144, 32768}, "provider.context_limit must not be negative"},
		{[2]int{0, -1}, "provider.output_limit must not be negative"},
		{[2]int{262144, -1}, "provider.output_limit must not be negative"},
		{[2]int{0, 32768}, "provider.output_limit requires provider.context_limit"},
		{[2]int{32768, 32768}, "provider.output_limit must be smaller than provider.context_limit"},
		{[2]int{32768, 65536}, "provider.output_limit must be smaller than provider.context_limit"},
	} {
		input := DefaultOptions()
		input.BaseURL = "http://example.invalid/v1"
		input.ContextLimit, input.OutputLimit = test.limits[0], test.limits[1]
		if err := input.validate(); err == nil || err.Error() != test.want {
			t.Errorf("limits %v returned %v, want %q", test.limits, err, test.want)
		}
		// Launch preparation must reject them before any command runs.
		if _, err := getResolvedOptions(input); err == nil || err.Error() != test.want {
			t.Errorf("resolved limits %v returned %v, want %q", test.limits, err, test.want)
		}
	}
}

func TestModelLimitDefaultsOutput(t *testing.T) {
	for _, test := range []struct{ context, output, wantContext, wantOutput int }{
		{0, 0, 0, 0},
		{262144, 0, 262144, 32768},
		{131072, 0, 131072, 32768},
		{65536, 0, 65536, 16384},
		{1000, 0, 1000, 250},
		{262144, 8192, 262144, 8192},
		{262144, 65536, 262144, 65536},
	} {
		input := Options{ContextLimit: test.context, OutputLimit: test.output}
		if context, output := input.modelLimit(); context != test.wantContext || output != test.wantOutput {
			t.Errorf("limits (%d, %d) resolved to (%d, %d), want (%d, %d)", test.context, test.output, context, output, test.wantContext, test.wantOutput)
		}
	}
}
