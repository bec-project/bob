package containermanager

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var resourceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// MCPServer configures an HTTP(S) MCP endpoint, optionally launched on the host.
type MCPServer struct {
	// Port optionally publishes a container TCP port on host loopback.
	Port int
	URL  string
	// HostCommand starts a foreground HTTP MCP server outside the container.
	HostCommand []string
}

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
	BindAddress string
	MountLabel  string
	// MCPServers maps OpenCode server names to their connection settings.
	MCPServers map[string]MCPServer
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
	if input.MountLabel != "" && input.MountLabel != "shared" && input.MountLabel != "private" {
		return errors.New("workspace.mount_label must be shared or private, or omitted for pre-labeled mounts")
	}
	if _, err := getEndpoint(input.BaseURL); err != nil {
		return err
	}
	ports := map[int]string{}
	for name, server := range input.MCPServers {
		if strings.TrimSpace(name) == "" || strings.ContainsRune(name, '\x00') {
			return errors.New("MCP server names must be nonempty and cannot contain NUL bytes")
		}
		if server.Port != 0 {
			if previous, exists := ports[server.Port]; exists {
				return fmt.Errorf("MCP servers %q and %q use the same container port %d", previous, name, server.Port)
			}
			ports[server.Port] = name
		}
		if err := server.validate(); err != nil {
			return fmt.Errorf("mcp.%s: %w", name, err)
		}
	}
	if input.BindAddress != "" && net.ParseIP(input.BindAddress) == nil {
		return errors.New("container.bind_address must be an IP address")
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
	return nil
}

func (server MCPServer) validate() error {
	if server.Port != 0 && (server.Port < 1024 || server.Port > 65535 || server.Port == 4096) {
		return errors.New("MCP port must be between 1024 and 65535, excluding OpenCode port 4096")
	}
	if server.URL != "" {
		if _, err := getEndpoint(server.URL); err != nil {
			return err
		}
	}
	if server.Port == 0 && server.URL == "" {
		return errors.New("MCP server requires url or a published port")
	}
	if len(server.HostCommand) > 0 {
		if server.URL == "" {
			return errors.New("host_command requires url")
		}
		if _, err := hostMCPAddress(server); err != nil {
			return err
		}
	}
	if len(server.HostCommand) > 0 && strings.TrimSpace(server.HostCommand[0]) == "" {
		return errors.New("host_command requires an executable")
	}
	for _, argument := range server.HostCommand {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("host_command arguments cannot contain NUL bytes")
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
	launchID   string
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
	// Resolved launch settings must not mutate the caller's map or argument arrays.
	options.MCPServers = make(map[string]MCPServer, len(input.MCPServers))
	for name, server := range input.MCPServers {
		options.MCPServers[name] = server
	}
	for name, server := range options.MCPServers {
		if len(server.HostCommand) == 0 {
			continue
		}
		directory, err := os.Getwd()
		if err != nil {
			return resolvedOptions{}, err
		}
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return resolvedOptions{}, err
		}
		executable, err := exec.LookPath(server.HostCommand[0])
		if err != nil {
			return resolvedOptions{}, fmt.Errorf("mcp.%s host executable: %w", name, err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return resolvedOptions{}, err
		}
		resolvedExecutable, err := filepath.EvalSymlinks(executable)
		if err != nil {
			return resolvedOptions{}, err
		}
		if err := validateHostExecutableChain(executable, options.mounts); err != nil {
			return resolvedOptions{}, fmt.Errorf("mcp.%s host executable: %w", name, err)
		}
		for _, mount := range options.mounts {
			if mount.readOnly {
				continue
			}
			writable, err := filepath.EvalSymlinks(mount.path)
			if err != nil {
				return resolvedOptions{}, err
			}
			if pathsOverlap(directory, mount.path) || pathsOverlap(resolved, writable) {
				return resolvedOptions{}, fmt.Errorf("mcp.%s host command launch directory must not overlap writable agent mounts", name)
			}
			if pathsOverlap(executable, mount.path) || pathsOverlap(resolvedExecutable, writable) {
				return resolvedOptions{}, fmt.Errorf("mcp.%s host executable must not overlap writable agent mounts", name)
			}
		}
		// Do not repeat PATH lookup after engine preparation: an agent could
		// create a shadow executable in an earlier, writable PATH directory.
		server.HostCommand = append([]string(nil), server.HostCommand...)
		server.HostCommand[0] = executable
		options.MCPServers[name] = server
	}
	options.workdir = options.mounts[0].path
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return resolvedOptions{}, err
	}
	options.password = hex.EncodeToString(token[:])
	if _, err := rand.Read(token[:]); err != nil {
		return resolvedOptions{}, err
	}
	options.launchID = hex.EncodeToString(token[:])
	if options.Name == "" {
		options.Name = fmt.Sprintf("bob-%d-%s", os.Getuid(), options.password[:8])
	}
	return options, nil
}

// validateHostExecutableChain checks intermediate symlinks as well as the final
// target. Retaining a safe lexical path preserves virtualenv interpreter identity.
func validateHostExecutableChain(executable string, mounts []mount) error {
	var writable []string
	for _, mount := range mounts {
		if !mount.readOnly {
			resolved, err := filepath.EvalSymlinks(mount.path)
			if err != nil {
				return err
			}
			writable = append(writable, mount.path, resolved)
		}
	}
	parts := strings.Split(executable, string(filepath.Separator))
	current := string(filepath.Separator)
	links := 0
	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			current = filepath.Dir(current)
			continue
		}
		candidate := filepath.Join(current, part)
		for _, directory := range writable {
			relative, err := filepath.Rel(directory, candidate)
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return errors.New("host executable must not overlap writable agent mounts, including intermediate symlinks")
			}
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		links++
		if links > 255 {
			return errors.New("too many host executable symlinks")
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			return err
		}
		if filepath.IsAbs(target) {
			current = string(filepath.Separator)
		}
		parts = append(strings.Split(target, string(filepath.Separator)), parts...)
	}
	return nil
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
	if err := validateMountDestination(path); err != nil {
		return "", err
	}
	return path, nil
}

func validateMountDestination(path string) error {
	if path == "/tmp" {
		return errors.New("/tmp is reserved for the container's temporary filesystem")
	}
	if pathsOverlap(path, "/home/opencode") {
		return errors.New("bind mounts must not overlap the container home volume")
	}
	// Include destination aliases in the bundled Alpine image. Resolving host
	// symlinks alone cannot detect aliases in the container's filesystem.
	for _, protected := range []string{"/usr", "/etc", "/bin", "/sbin", "/lib", "/lib64", "/proc", "/sys", "/dev", "/run", "/var/run", "/var/lock"} {
		if pathsOverlap(path, protected) {
			return errors.New("bind mounts must not overlap container system directories")
		}
	}
	return nil
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
