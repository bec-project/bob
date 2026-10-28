package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const pinnedOpenCodeVersion = "1.18.35"

// validateNodeVersion checks the selected runtime before it executes SRT. Do not inherit
// NODE_OPTIONS, preload settings or other host environment while running this host-side check.
func validateNodeVersion(parent context.Context, node string) error {
	version, err := sandboxRuntimeVersion(parent, node)
	if err != nil {
		return fmt.Errorf("check Node.js version at %s (requires 22.12 or later): %w", node, err)
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	var numbers [3]int
	valid := len(parts) == len(numbers)
	if valid {
		for index, part := range parts {
			if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				valid = false
				break
			}
			numbers[index], err = strconv.Atoi(part)
			if err != nil {
				valid = false
				break
			}
		}
	}
	if !valid || numbers[0] < 22 || numbers[0] == 22 && numbers[1] < 12 {
		return fmt.Errorf("Node.js at %s reports %q; sandbox-runtime requires a stable Node.js 22.12 or later", node, version)
	}
	return nil
}

// validateOpenCodeVersion ties configuration-discovery protections to the release tested with
// them. Upgrading OpenCode requires rerunning the executable policy and Linux sandbox tests.
func validateOpenCodeVersion(ctx context.Context, executable string) error {
	version, err := sandboxRuntimeVersion(ctx, executable)
	if err != nil {
		return fmt.Errorf("check OpenCode version: %w", err)
	}
	if version != pinnedOpenCodeVersion {
		return fmt.Errorf("OpenCode %q is unsupported by this sandbox policy; install the pinned release %s", version, pinnedOpenCodeVersion)
	}
	return nil
}

func sandboxRuntimeVersion(parent context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Dir = filepath.Dir(executable)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.WaitDelay = time.Second
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

// validateSandboxExecutables keeps host-executed launch code outside every agent-writable
// directory, including intermediate symlinks. Explicit development installations remain usable
// without trusting an executable left by an earlier sandbox in its persistent storage.
func validateSandboxExecutables(options resolvedOptions, helper, openCode string, tools sandboxTools) error {
	writable := []string{options.Home}
	for _, tool := range []struct{ name, path string }{
		{"bob-manager", helper}, {"OpenCode", openCode}, {"Node.js", tools.node},
		{"sandbox-runtime", tools.srt}, {"bubblewrap", tools.bwrap}, {"socat", tools.socat},
		{"bash", tools.bash}, {"ripgrep", tools.ripgrep}, {"seccomp helper", tools.applySeccomp},
	} {
		if tool.path == "" {
			continue
		}
		if err := validateHostExecutableChain(tool.path, writable); err != nil {
			return fmt.Errorf("%s must stay outside sandbox.home: %w", tool.name, err)
		}
	}
	// SRT runs on the host and imports JavaScript and npm dependencies. Protect its entire
	// installation, not only cli.js: a writable sibling module is executable launch code too.
	if tools.srt != "" {
		installation := filepath.Dir(filepath.Dir(tools.srt))
		for directory := installation; directory != filepath.Dir(directory); directory = filepath.Dir(directory) {
			if filepath.Base(directory) == "node_modules" {
				installation = directory
			}
		}
		if err := validateSandboxReadOnlyTree(installation, writable); err != nil {
			return fmt.Errorf("sandbox-runtime and its dependencies must stay outside writable agent storage: %w", err)
		}
	}
	return nil
}

func validateSandboxReadOnlyTree(directory string, directories []string) error {
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	for _, directoryPath := range directories {
		writable, err := filepath.EvalSymlinks(directoryPath)
		if err != nil {
			return err
		}
		if pathsOverlap(directory, directoryPath) || pathsOverlap(resolved, writable) {
			return fmt.Errorf("writable directory %s overlaps the runtime installation", directoryPath)
		}
	}
	return nil
}
