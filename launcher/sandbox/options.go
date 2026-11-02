package sandbox

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
	"sort"
	"strconv"
	"strings"
)

// pluginName is how GUIs name their plugins.
var pluginName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// MCPServer configures an HTTP(S) MCP endpoint, optionally launched on the host.
type MCPServer struct {
	URL string
	// HostCommand starts a foreground HTTP MCP server outside the sandbox.
	HostCommand []string
	// Plugin names the GUI plugin that adds views for this server's tools; empty for none.
	Plugin string
}

type Options struct {
	BaseURL     string
	Model       string
	APIKey      string
	BindAddress string
	// ContextLimit is the model's context window in tokens. OpenCode uses it for its
	// context meter and automatic compaction. Zero leaves OpenCode's model limits unset.
	ContextLimit int
	// OutputLimit is the model's maximum output in tokens. It requires ContextLimit and
	// must be smaller than it. Zero selects min(32768, ContextLimit/4).
	OutputLimit int
	// MCPServers maps OpenCode server names to their connection settings.
	MCPServers map[string]MCPServer
	// OpenCode is the OpenCode executable the sandbox runs; a bare name is looked up on PATH.
	// Empty selects the OpenCode installed with bob-manager, else opencode on PATH.
	OpenCode string
	// Home is the folder that keeps OpenCode's home, configuration and sessions.
	Home string
	// SandboxRuntime is sandbox-runtime's cli.js. Empty selects the copy installed with
	// bob-manager (lib/sandbox-runtime).
	SandboxRuntime string
}

// defaultOutputLimit caps the output limit derived from ContextLimit; OpenCode's model
// limit requires an output value whenever it receives a context window.
const defaultOutputLimit = 32768

// DefaultOptions returns the launcher's configuration defaults.
// Callers must supply a provider URL and, when needed, an API key.
func DefaultOptions() Options {
	return Options{
		Model:       "Qwen3.8-27B",
		BindAddress: "127.0.0.1",
		Home:        defaultSandboxHome(),
	}
}

func (input Options) validate() error {
	if _, err := getEndpoint(input.BaseURL); err != nil {
		return err
	}
	for name, server := range input.MCPServers {
		if strings.TrimSpace(name) == "" || strings.ContainsRune(name, '\x00') {
			return errors.New("MCP server names must be nonempty and cannot contain NUL bytes")
		}
		if err := server.validate(); err != nil {
			return fmt.Errorf("mcp.%s: %w", name, err)
		}
	}
	if input.BindAddress != "" && net.ParseIP(input.BindAddress) == nil {
		return errors.New("sandbox.bind_address must be an IP address")
	}
	if input.Model == "" {
		return errors.New("enter a model ID")
	}
	if input.ContextLimit < 0 {
		return errors.New("provider.context_limit must not be negative")
	}
	if input.OutputLimit < 0 {
		return errors.New("provider.output_limit must not be negative")
	}
	if input.OutputLimit != 0 && input.ContextLimit == 0 {
		return errors.New("provider.output_limit requires provider.context_limit")
	}
	if input.OutputLimit != 0 && input.OutputLimit >= input.ContextLimit {
		return errors.New("provider.output_limit must be smaller than provider.context_limit")
	}
	return nil
}

// modelLimit returns the context and output token limits passed to OpenCode, or zeros
// when no context limit is configured.
func (input Options) modelLimit() (int, int) {
	if input.ContextLimit <= 0 {
		return 0, 0
	}
	if input.OutputLimit > 0 {
		return input.ContextLimit, input.OutputLimit
	}
	return input.ContextLimit, min(defaultOutputLimit, input.ContextLimit/4)
}

func (server MCPServer) validate() error {
	if server.URL == "" {
		return errors.New("MCP server requires url")
	}
	if _, err := getEndpoint(server.URL); err != nil {
		return err
	}
	if server.Plugin != "" && !pluginName.MatchString(server.Plugin) {
		return errors.New("plugin must be a lowercase name such as bec")
	}
	if len(server.HostCommand) > 0 {
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
	// binary is bwrap.
	binary   string
	password string
	// name tells this launch apart from the next.
	name string
}

func getResolvedOptions(input Options) (resolvedOptions, error) {
	if input.BindAddress == "" {
		input.BindAddress = DefaultOptions().BindAddress
	}
	if input.Home == "" {
		input.Home = DefaultOptions().Home
	}
	if err := input.validate(); err != nil {
		return resolvedOptions{}, err
	}
	options := resolvedOptions{Options: input}
	var err error
	if options.binary, err = findBwrap(); err != nil {
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
		executable, err := exec.LookPath(server.HostCommand[0])
		if err != nil {
			return resolvedOptions{}, fmt.Errorf("mcp.%s host executable: %w", name, err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return resolvedOptions{}, err
		}
		// Do not repeat PATH lookup after sandbox preparation: an agent could
		// create a shadow executable in an earlier, writable PATH directory.
		server.HostCommand = append([]string(nil), server.HostCommand...)
		server.HostCommand[0] = executable
		options.MCPServers[name] = server
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return resolvedOptions{}, err
	}
	options.password = hex.EncodeToString(token[:])
	options.name = fmt.Sprintf("bob-%d-%s", os.Getuid(), options.password[:8])
	return options, nil
}

// validateHostExecutableChain checks intermediate symlinks as well as the final
// target. Retaining a safe lexical path preserves virtualenv interpreter identity.
func validateHostExecutableChain(executable string, directories []string) error {
	var writable []string
	for _, directory := range directories {
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return err
		}
		writable = append(writable, directory, resolved)
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
				return errors.New("host executable must not overlap writable sandbox data, including intermediate symlinks")
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

func pathsOverlap(a, b string) bool {
	separator := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, separator)+separator) || strings.HasPrefix(b, strings.TrimSuffix(a, separator)+separator)
}

func mcpServerNames(servers map[string]MCPServer) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
