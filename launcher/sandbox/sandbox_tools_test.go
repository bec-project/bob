package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeVersionPreflight(t *testing.T) {
	for _, version := range []string{"v22.12.0", "v22.23.2", "v24.0.0", "v25.1.0", "v20.19.0", "v22.11.0", "v22.12.0-nightly", "v22.12", "v-23.0.0", "unexpected"} {
		t.Run(version, func(t *testing.T) {
			node := filepath.Join(t.TempDir(), "node")
			// The version check must not inherit code-loading configuration from the operator.
			t.Setenv("NODE_OPTIONS", "--require=/private/operator-preload.js")
			script := "#!/bin/sh\n[ -z \"${NODE_OPTIONS+x}\" ] || exit 9\n[ \"$1\" = --version ] || exit 10\nprintf '%s\\n' '" + version + "'\n"
			if err := os.WriteFile(node, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			err := validateNodeVersion(context.Background(), node)
			wantOK := version == "v22.12.0" || version == "v22.23.2" || version == "v24.0.0" || version == "v25.1.0"
			if (err == nil) != wantOK {
				t.Fatalf("version preflight = %v, accepted = %v", err, wantOK)
			}
		})
	}
}

func TestNodeVersionPreflightReportsExecutionFailure(t *testing.T) {
	if err := validateNodeVersion(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted a missing Node.js executable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateNodeVersion(ctx, "/bin/sh"); err == nil {
		t.Fatal("ignored cancelled startup")
	}
}

func TestOpenCodeVersionMatchesTheTestedPolicy(t *testing.T) {
	for _, version := range []string{pinnedOpenCodeVersion, "1.18.34", "1.18.36", "unexpected"} {
		executable := filepath.Join(t.TempDir(), "opencode")
		if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' '"+version+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		err := validateOpenCodeVersion(context.Background(), executable)
		if (err == nil) != (version == pinnedOpenCodeVersion) {
			t.Fatalf("OpenCode %s: %v", version, err)
		}
	}
}

func TestHostRuntimeCannotComeFromWritableAgentStorage(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	trusted := filepath.Join(root, "trusted")
	for _, directory := range []string{home, trusted} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "node"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	options := resolvedOptions{Options: Options{Home: home}}
	for _, directory := range []string{home, trusted} {
		err := validateSandboxExecutables(options, "", "", sandboxTools{node: filepath.Join(directory, "node")})
		if (err == nil) != (directory == trusted) {
			t.Fatalf("runtime from %s: %v", directory, err)
		}
	}
	alias := filepath.Join(home, "safe-looking-node")
	if err := os.Symlink(filepath.Join(trusted, "node"), alias); err != nil {
		t.Fatal(err)
	}
	if err := validateSandboxExecutables(options, "", "", sandboxTools{node: alias}); err == nil || !strings.Contains(err.Error(), "intermediate symlinks") {
		t.Fatalf("accepted an agent-controlled runtime alias: %v", err)
	}
}

func TestHostRuntimeDependenciesCannotBeMountedWritable(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	modules := filepath.Join(root, "node_modules")
	cli := filepath.Join(modules, "@anthropic-ai", "sandbox-runtime", "dist", "cli.js")
	dependency := filepath.Join(modules, "zod")
	for _, directory := range []string{home, filepath.Dir(cli), dependency} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cli, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	options := resolvedOptions{Options: Options{Home: home}}
	if err := validateSandboxExecutables(options, "", "", sandboxTools{srt: cli}); err != nil {
		t.Fatal(err)
	}
	options.Home = dependency
	if err := validateSandboxExecutables(options, "", "", sandboxTools{srt: cli}); err == nil {
		t.Fatal("accepted a writable sibling npm dependency")
	}
	options.Home = filepath.Dir(cli)
	if err := validateSandboxExecutables(options, "", "", sandboxTools{srt: cli}); err == nil {
		t.Fatal("accepted writable SRT JavaScript modules")
	}
}
