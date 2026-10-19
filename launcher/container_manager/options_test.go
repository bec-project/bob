package containermanager

import (
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMountLabelValidation(t *testing.T) {
	input := DefaultOptions()
	input.BaseURL = "https://provider.example/"
	for _, policy := range []string{"", "shared", "private"} {
		input.MountLabel = policy
		if err := input.validate(); err != nil {
			t.Fatal(err)
		}
	}
	input.MountLabel = "invalid"
	if err := input.validate(); err == nil {
		t.Fatal("accepted invalid mount labeling policy")
	}
}

func TestMCPPort(t *testing.T) {
	for _, port := range []int{0, 1024, 19000, 65535} {
		input := DefaultOptions()
		input.BaseURL = "https://example.invalid/agent/"
		input.MCPServers = map[string]MCPServer{"bob": {Port: port, URL: "http://example.invalid/mcp"}}
		if err := input.validate(); err != nil {
			t.Fatalf("rejected valid port %d: %v", port, err)
		}
	}
	for _, port := range []int{-1, 1, 4096, 65536} {
		input := DefaultOptions()
		input.BaseURL = "https://example.invalid/agent/"
		input.MCPServers = map[string]MCPServer{"bob": {Port: port, URL: "http://example.invalid/mcp"}}
		if err := input.validate(); err == nil {
			t.Fatalf("accepted invalid port %d", port)
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

func TestMountsProtectContainerHome(t *testing.T) {
	for _, path := range []string{"/", "/home", "/home/opencode", "/home/opencode/notes"} {
		if !pathsOverlap(path, "/home/opencode") {
			t.Fatalf("missed home overlap: %s", path)
		}
	}
	if _, err := getDirectory("/"); err == nil {
		t.Fatal("accepted a mount that hides the home volume")
	}
	if pathsOverlap("/home/other", "/home/opencode") {
		t.Fatal("rejected an unrelated home directory")
	}
}

func TestMountDestinationsProtectTemporaryFilesystemAndImageAliases(t *testing.T) {
	for _, path := range []string{"/tmp", "/var", "/var/run", "/var/run/docker", "/var/lock", "/var/lock/agent", "/lib64", "/lib64/agent"} {
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

func TestOpenCodeConfigDir(t *testing.T) {
	newConfigDir := func(parent string) string {
		t.Helper()
		directory := filepath.Join(parent, "opencode")
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, ".gitignore"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		return directory
	}
	workdir := t.TempDir()
	workspace := []mount{{path: workdir}}
	configDir := newConfigDir(t.TempDir())
	if path, err := getOpenCodeConfigDir("", workspace); err != nil || path != "" {
		t.Fatalf("unset config_dir: %q, %v", path, err)
	}
	if path, err := getOpenCodeConfigDir(configDir, workspace); err != nil || path != configDir {
		t.Fatalf("valid config_dir: %q, %v", path, err)
	}
	// Read-only mounts may share it: the agent cannot change it through them.
	if _, err := getOpenCodeConfigDir(configDir, append(workspace, mount{path: filepath.Dir(configDir), readOnly: true})); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ignoreDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(ignoreDirectory, ".gitignore"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(workdir, alias); err != nil {
		t.Fatal(err)
	}
	inWorkspace := newConfigDir(workdir)
	for name, test := range map[string]struct {
		directory string
		mounts    []mount
	}{
		"missing":                       {filepath.Join(t.TempDir(), "missing"), workspace},
		"a file":                        {file, workspace},
		"without .gitignore":            {t.TempDir(), workspace},
		".gitignore is a directory":     {ignoreDirectory, workspace},
		"inside the workspace":          {inWorkspace, workspace},
		"inside the workspace by alias": {filepath.Join(alias, "opencode"), workspace},
		"containing the workspace":      {filepath.Dir(configDir), []mount{{path: configDir}}},
		"inside a writable mount":       {configDir, append(workspace, mount{path: filepath.Dir(configDir)})},
	} {
		if _, err := getOpenCodeConfigDir(test.directory, test.mounts); err == nil {
			t.Errorf("accepted a config_dir %s", name)
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
	input.MCPServers = map[string]MCPServer{"bec_mcp": {URL: "http://example.invalid/mcp"}}
	if err := input.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMCPPortsMustBeUnique(t *testing.T) {
	input := DefaultOptions()
	input.BaseURL = "http://example.invalid/v1"
	input.MCPServers = map[string]MCPServer{
		"first":  {Port: 19000},
		"second": {Port: 19000},
	}
	if err := input.validate(); err == nil {
		t.Fatal("accepted duplicate published container ports")
	}
	input.MCPServers["second"] = MCPServer{Port: 19001}
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

func TestDefaultHomeVolumeBelongsToTheUser(t *testing.T) {
	volume := DefaultOptions().HomeVolume
	if !resourceName.MatchString(volume) || !strings.HasPrefix(volume, "bob-") || !strings.HasSuffix(volume, "-home") {
		t.Fatalf("default home volume %q is not a per-user volume name", volume)
	}
	if current, err := user.Current(); err == nil && current.Username != "" && resourceName.MatchString(current.Username) &&
		volume != "bob-"+current.Username+"-home" {
		t.Fatalf("default home volume %q does not name user %s", volume, current.Username)
	}
}
