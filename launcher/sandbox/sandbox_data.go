package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
)

// sandboxDataPaths are the only persistent directories exposed writable. The parent home is
// not mounted: it can contain old executable configuration that OpenCode's v2 loader discovers.
func sandboxDataPaths(home string) []string {
	return []string{filepath.Join(home, ".local", "share"), filepath.Join(home, ".cache"), filepath.Join(home, ".local", "state")}
}

func prepareSandboxData(home string) (string, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", fmt.Errorf("create private sandbox storage: %w", err)
	}
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("sandbox.home must be a private directory (mode 0700): %s", home)
	}
	for _, relative := range []string{".local", filepath.Join(".local", "share"), ".cache", filepath.Join(".local", "state"), filepath.Join(".cache", "tmp")} {
		directory := filepath.Join(home, relative)
		if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
			return "", fmt.Errorf("create sandbox data directory: %w", err)
		}
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("sandbox data directory must not be a symbolic link or file: %s", directory)
		}
	}
	return home, nil
}

// Host MCP code stays on the host, outside all agent state. Resolve executables once, but
// validate their symlink chains and launch directory after the home directory exists.
func validateHostMCPStorage(options resolvedOptions) error {
	writable := []string{options.Home}
	for name, server := range options.MCPServers {
		if len(server.HostCommand) == 0 {
			continue
		}
		if err := validateHostExecutableChain(server.HostCommand[0], writable); err != nil {
			return fmt.Errorf("mcp.%s host executable must stay outside sandbox.home: %w", name, err)
		}
		directory, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := validateHostExecutableChain(directory, writable); err != nil {
			return fmt.Errorf("mcp.%s host launch directory must stay outside sandbox.home: %w", name, err)
		}
	}
	return nil
}
