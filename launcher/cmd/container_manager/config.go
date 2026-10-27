package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"bob/container_manager"

	"github.com/pelletier/go-toml/v2"
)

// config describes the TOML sections; the manager receives plain Options.
type config struct {
	Sandbox struct {
		OpenCode    string `toml:"opencode"`
		Home        string `toml:"home"`
		BindAddress string `toml:"bind_address"`
		Runtime     string `toml:"runtime"`
	} `toml:"sandbox"`
	// Decode only to explain the removed section; bob no longer runs in a container.
	UnsupportedContainer any `toml:"container"`
	Workspace            struct {
		Workdir string   `toml:"workdir"`
		RW      []string `toml:"rw"`
		RO      []string `toml:"ro"`
		// Decode only to explain the removed setting.
		UnsupportedMountLabel any `toml:"mount_label"`
	} `toml:"workspace"`
	Provider struct {
		BaseURL      string  `toml:"base_url"`
		Model        string  `toml:"model"`
		Token        *string `toml:"token"`
		ContextLimit int     `toml:"context_limit"`
		OutputLimit  int     `toml:"output_limit"`
	} `toml:"provider"`
	MCP map[string]struct {
		URL         string   `toml:"url"`
		HostCommand []string `toml:"host_command"`
		Plugin      string   `toml:"plugin"`
		// Decode only to explain the removed settings; never forward or execute them.
		UnsupportedCommand any `toml:"command"`
		UnsupportedPort    any `toml:"port"`
	} `toml:"mcp"`
}

func loadConfig(path string) (containermanager.Options, error) {
	input := containermanager.DefaultOptions()
	var settings config
	settings.Sandbox.BindAddress = input.BindAddress
	settings.Sandbox.OpenCode = input.OpenCode
	settings.Sandbox.Home = input.Home
	settings.Provider.Model = input.Model
	file, err := os.Open(path)
	if err != nil {
		return input, err
	}
	defer file.Close()

	decoder := toml.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return input, fmt.Errorf("read config %s: %w", path, err)
	}
	if settings.UnsupportedContainer != nil {
		return input, errors.New("bob no longer runs in a container: replace [container] with [sandbox] (opencode, home, bind_address); see deploy/bob.example.toml")
	}
	if settings.Workspace.UnsupportedMountLabel != nil {
		return input, errors.New("workspace.mount_label was for containers; remove it")
	}
	input = containermanager.Options{
		Workdir:        settings.Workspace.Workdir,
		ReadWrite:      settings.Workspace.RW,
		ReadOnly:       settings.Workspace.RO,
		BaseURL:        settings.Provider.BaseURL,
		Model:          settings.Provider.Model,
		ContextLimit:   settings.Provider.ContextLimit,
		OutputLimit:    settings.Provider.OutputLimit,
		BindAddress:    settings.Sandbox.BindAddress,
		OpenCode:       settings.Sandbox.OpenCode,
		Home:           settings.Sandbox.Home,
		SandboxRuntime: settings.Sandbox.Runtime,
		APIKey:         os.Getenv("API_KEY"),
	}
	if len(settings.MCP) > 0 {
		input.MCPServers = make(map[string]containermanager.MCPServer, len(settings.MCP))
		for name, server := range settings.MCP {
			if server.UnsupportedCommand != nil {
				return input, fmt.Errorf("mcp.%s.command is unsupported; use url with host_command for host startup", name)
			}
			if server.UnsupportedPort != nil {
				return input, fmt.Errorf("mcp.%s.port published a container port; the sandbox has none, so remove it", name)
			}
			input.MCPServers[name] = containermanager.MCPServer{
				URL:         server.URL,
				HostCommand: server.HostCommand,
				Plugin:      server.Plugin,
			}
		}
	}
	if settings.Provider.Token != nil {
		input.APIKey = *settings.Provider.Token
	}
	if input.BaseURL == "" {
		return input, errors.New("config requires provider.base_url")
	}
	if endpoint, err := url.Parse(input.BaseURL); err == nil && endpoint.Hostname() == "provider.example" {
		return input, errors.New("provider.base_url is a placeholder; replace it with your actual provider or gateway API URL")
	}

	// Resolve paths beside the config so it works from any launching directory.
	if input.Workdir != "" && !filepath.IsAbs(input.Workdir) {
		input.Workdir = filepath.Join(filepath.Dir(path), input.Workdir)
	}
	if input.Home != "" && !filepath.IsAbs(input.Home) {
		input.Home = filepath.Join(filepath.Dir(path), input.Home)
	}
	for _, directories := range [][]string{input.ReadWrite, input.ReadOnly} {
		for i, directory := range directories {
			if directory != "" && !filepath.IsAbs(directory) {
				directories[i] = filepath.Join(filepath.Dir(path), directory)
			}
		}
	}
	return input, nil
}
