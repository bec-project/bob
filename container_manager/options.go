package containermanager

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var resourceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type Options struct {
	Web         bool
	Image       string
	HomeVolume  string
	Name        string
	Runtime     string
	Workdir     string
	ReadWrite   []string
	ReadOnly    []string
	BaseURL     string
	Model       string
	APIKey      string
	MCPPort     int
	MCPCommand  []string
	MCPURL      string
	BindAddress string
}

// DefaultOptions returns the launcher's configuration defaults.
// Callers must supply a working directory, a provider URL, and, when needed, an API key.
func DefaultOptions() Options {
	return Options{
		Runtime:     "podman",
		Image:       "localhost/bob",
		HomeVolume:  "bob-cli-home",
		Model:       "Qwen3.8-27B",
		BindAddress: "0.0.0.0",
	}
}

func (input Options) validate() error {
	if _, err := getEndpoint(input.BaseURL); err != nil {
		return err
	}
	if input.MCPURL != "" {
		if _, err := getEndpoint(input.MCPURL); err != nil {
			return err
		}
	}
	if input.BindAddress != "" && net.ParseIP(input.BindAddress) == nil {
		return errors.New("container.bind_address must be an IP address")
	}
	if input.MCPURL != "" && len(input.MCPCommand) > 0 {
		return errors.New("choose mcp.url or mcp.command, not both")
	}
	if input.MCPPort != 0 && (input.MCPPort < 1024 || input.MCPPort > 65535 || input.MCPPort == 4096) {
		return errors.New("MCP port must be between 1024 and 65535, excluding OpenCode port 4096")
	}
	if input.Model == "" {
		return errors.New("enter a model ID")
	}
	if !resourceName.MatchString(input.HomeVolume) {
		return errors.New("a valid named home volume is required")
	}
	if input.Name != "" && !resourceName.MatchString(input.Name) {
		return errors.New("invalid container name")
	}
	if len(input.MCPCommand) > 0 && strings.TrimSpace(input.MCPCommand[0]) == "" {
		return errors.New("MCP command requires an executable")
	}
	for _, argument := range input.MCPCommand {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("MCP command arguments cannot contain NUL bytes")
		}
	}
	return nil
}

func getEndpoint(value string) (*url.URL, error) {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, fmt.Errorf("endpoint must be an HTTP(S) URL without credentials or a fragment")
	}
	if endpoint.Port() != "" {
		port, err := strconv.Atoi(endpoint.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("endpoint port must be between 1 and 65535")
		}
	}
	return endpoint, nil
}

// resolvedOptions holds the configuration prepared for starting a backend.
type resolvedOptions struct {
	Options
	workdir    string
	binary     string
	mounts     []mount
	userConfig userSettings
	password   string
}

func getResolvedOptions(input Options) (resolvedOptions, error) {
	if input.Image == "" {
		input.Image = DefaultOptions().Image
	}
	if input.BindAddress == "" {
		input.BindAddress = DefaultOptions().BindAddress
	}
	if err := input.validate(); err != nil {
		return resolvedOptions{}, err
	}
	options := resolvedOptions{Options: input}
	var err error
	options.binary, err = getExecutable(input.Runtime)
	if err != nil {
		return resolvedOptions{}, err
	}
	options.mounts, err = getMounts(options.Options)
	if err != nil {
		return resolvedOptions{}, err
	}
	options.workdir = options.mounts[0].path
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return resolvedOptions{}, err
	}
	options.password = hex.EncodeToString(token[:])
	if options.Name == "" {
		options.Name = fmt.Sprintf("bob-%d-%s", os.Getuid(), options.password[:8])
	}
	return options, nil
}

type mount struct {
	path     string
	readOnly bool
}

func getDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("choose a mount directory")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}
	if strings.ContainsAny(path, ",:\n") {
		return "", errors.New("mount paths cannot contain commas, colons, or newlines")
	}
	if pathsOverlap(path, "/home/opencode") {
		return "", errors.New("bind mounts must not overlap the container home volume")
	}
	for _, protected := range []string{"/usr", "/etc", "/bin", "/sbin", "/lib", "/proc", "/sys", "/dev", "/run"} {
		if pathsOverlap(path, protected) {
			return "", errors.New("bind mounts must not overlap container system directories")
		}
	}
	return path, nil
}

func getMounts(input Options) ([]mount, error) {
	var mounts []mount
	var resolvedPaths []string
	groups := []struct {
		paths    []string
		readOnly bool
	}{
		{paths: []string{input.Workdir}},
		{paths: input.ReadWrite},
		{paths: input.ReadOnly, readOnly: true},
	}
	for _, group := range groups {
		for _, directory := range group.paths {
			path, err := getDirectory(directory)
			if err != nil {
				return nil, err
			}
			// Resolve symlinks to avoid overlapping paths due to symlinks
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil, err
			}
			duplicate := false
			for i, previous := range mounts {
				// Ensure that read-only and writable directories do not overlap, as this can cause issues with container mounts.
				if group.readOnly != previous.readOnly &&
					(pathsOverlap(resolved, resolvedPaths[i]) || pathsOverlap(path, previous.path)) {
					return nil, errors.New("read-only and writable directories must not overlap")
				}
				if path == previous.path {
					duplicate = true
				}
			}
			if duplicate {
				continue
			}
			mounts = append(mounts, mount{path: path, readOnly: group.readOnly})
			resolvedPaths = append(resolvedPaths, resolved)
		}
	}
	return mounts, nil
}

func pathsOverlap(a, b string) bool {
	separator := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, separator)+separator) || strings.HasPrefix(b, strings.TrimSuffix(a, separator)+separator)
}
