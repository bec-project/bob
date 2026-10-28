package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxStorageKeepsExecutableConfigurationOutOfWritableData(t *testing.T) {
	home, err := prepareSandboxData(filepath.Join(t.TempDir(), "home"))
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range sandboxDataPaths(home) {
		if info, err := os.Stat(directory); err != nil || !info.IsDir() {
			t.Fatalf("persistent data directory not prepared: %s, %v", directory, err)
		}
		if pathsOverlap(directory, filepath.Join(home, ".opencode-policy")) {
			t.Fatal("writable data overlaps trusted OpenCode policy")
		}
	}
	if _, err := prepareSandboxData(home); err != nil {
		t.Fatalf("existing private storage cannot be reused: %v", err)
	}
}

func TestSandboxStorageRejectsSymlinkedDataAndPublicHome(t *testing.T) {
	for _, relative := range []string{".local", ".cache", filepath.Join(".local", "share"), filepath.Join(".local", "state"), filepath.Join(".cache", "tmp")} {
		t.Run(relative, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(home, relative)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			if _, err := prepareSandboxData(home); err == nil {
				t.Fatal("accepted a data alias into unrelated host storage")
			}
		})
	}
	home := t.TempDir()
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareSandboxData(home); err == nil {
		t.Fatal("accepted publicly readable sandbox state")
	}
}
