package containermanager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://example.com", "https://user:secret@example.com", "https://example.com:70000", "https://example.com:0", "https://example.com/#fragment"} {
		if _, err := getEndpoint(endpoint); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
}

func TestMountsPreservePathsAndReadOnly(t *testing.T) {
	workdir, writable, readOnly := t.TempDir(), t.TempDir(), t.TempDir()
	mounts, err := getMounts(Options{
		Workdir:   workdir,
		ReadWrite: []string{writable, workdir, writable},
		ReadOnly:  []string{readOnly, readOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []mount{{path: workdir}, {path: writable}, {path: readOnly, readOnly: true}}
	if !reflect.DeepEqual(mounts, want) {
		t.Fatalf("mounts: %v, want %v", mounts, want)
	}
	mounts, err = getMounts(Options{Workdir: workdir})
	if err != nil || len(mounts) != 1 || mounts[0] != (mount{path: workdir}) {
		t.Fatalf("unexpected dedicated working directory: %v, %v", mounts, err)
	}
}

func TestReadOnlyMountsHaveNoWritableAlias(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, input := range []Options{
		{Workdir: root, ReadOnly: []string{child}},
		{Workdir: child, ReadOnly: []string{root}},
		{Workdir: root, ReadOnly: []string{root}},
		{Workdir: root, ReadOnly: []string{alias}},
		{Workdir: alias, ReadOnly: []string{child}},
		{Workdir: t.TempDir(), ReadWrite: []string{root}, ReadOnly: []string{alias}},
	} {
		if _, err := getMounts(input); err == nil {
			t.Fatal("read-only source was reachable through a writable mount")
		}
	}
	if _, err := getMounts(Options{Workdir: t.TempDir(), ReadOnly: []string{root, child, alias}}); err != nil {
		t.Fatal(err)
	}
}

func TestMountsPreserveSymlinkAndRelativePaths(t *testing.T) {
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(t.TempDir(), alias); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(alias))
	mounts, err := getMounts(Options{Workdir: "alias"})
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs("alias")
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0] != (mount{path: absolute}) {
		t.Fatalf("symlink path remapped: %v", mounts)
	}
}

func TestMountsRequireWorkdir(t *testing.T) {
	for _, input := range []Options{{}, {ReadWrite: []string{t.TempDir()}}, {ReadOnly: []string{t.TempDir()}}, {Workdir: t.TempDir(), ReadOnly: []string{""}}} {
		if _, err := getMounts(input); err == nil {
			t.Fatal("accepted an empty mount path or missing workdir")
		}
	}
}

func TestMountsProtectTheSandboxHome(t *testing.T) {
	for _, path := range []string{"/", "/home", "/home/opencode", "/home/opencode/notes"} {
		if !pathsOverlap(path, "/home/opencode") {
			t.Fatalf("missed home overlap: %s", path)
		}
	}
	if _, err := getDirectory("/"); err == nil {
		t.Fatal("accepted a mount that hides the sandbox home")
	}
	if pathsOverlap("/home/other", "/home/opencode") {
		t.Fatal("rejected an unrelated home directory")
	}
}

func TestMountDestinationsProtectTheSandboxSystemDirectories(t *testing.T) {
	for _, path := range []string{"/tmp", "/run", "/run/bob", "/lib64", "/lib64/agent", "/bob", "/usr/local", "/etc/pki"} {
		if err := validateMountDestination(path); err == nil {
			t.Errorf("accepted protected destination %s", path)
		}
	}
	for _, path := range []string{"/tmp/workspace", "/var/tmp/workspace", "/workspace", "/var/runtime"} {
		if err := validateMountDestination(path); err != nil {
			t.Errorf("rejected unrelated destination %s: %v", path, err)
		}
	}
	if _, err := getMounts(Options{Workdir: "/tmp"}); err == nil {
		t.Fatal("accepted a workspace conflicting with the mandatory tmpfs")
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
		{URL: "http://host.docker.internal:19000/mcp", HostCommand: []string{" "}},
		{URL: "http://host.docker.internal:19000/mcp", HostCommand: []string{"python3", "bad\x00arg"}},
		{URL: "http://example.invalid/mcp", HostCommand: []string{"python3"}},
		{URL: "http://localhost:19000/mcp", HostCommand: []string{"python3"}},
		{URL: "http://127.0.0.1:19000/mcp", HostCommand: []string{"python3"}},
		{URL: "http://[::1]:19000/mcp", HostCommand: []string{"python3"}},
	} {
		if err := server.validate(); err == nil {
			t.Fatal("accepted invalid host MCP configuration")
		}
	}
	if err := (MCPServer{URL: "http://host.docker.internal:19000/mcp", HostCommand: []string{"python3"}}).validate(); err != nil {
		t.Fatalf("host command requires an unnecessary directory: %v", err)
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
		// Launch preparation must reject them before any engine command runs.
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
