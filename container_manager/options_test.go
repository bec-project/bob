package containermanager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateMCPCommand(t *testing.T) {
	for _, command := range [][]string{{""}, {"  "}, {"python3", "bad\x00argument"}} {
		input := DefaultOptions()
		input.BaseURL = "http://example.invalid/v1"
		input.MCPCommand = command
		if err := input.validate(); err == nil {
			t.Fatalf("accepted invalid MCP command %q", command)
		}
	}
}

func TestMCPPort(t *testing.T) {
	for _, port := range []int{0, 1024, 19000, 65535} {
		input := DefaultOptions()
		input.BaseURL = "https://example.invalid/agent/"
		input.MCPPort = port
		if err := input.validate(); err != nil {
			t.Fatalf("rejected valid port %d: %v", port, err)
		}
	}
	for _, port := range []int{-1, 1, 4096, 65536} {
		input := DefaultOptions()
		input.BaseURL = "https://example.invalid/agent/"
		input.MCPPort = port
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
