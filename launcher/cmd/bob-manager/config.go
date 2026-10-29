package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"bob/sandbox"

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
	} `toml:"mcp"`
}

func loadConfig(path string) (sandbox.Options, error) {
	input := sandbox.DefaultOptions()
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
		var unknown *toml.StrictMissingError
		if errors.As(err, &unknown) {
			return input, fmt.Errorf("read config %s: unknown settings:\n%s", path, unknown.String())
		}
		return input, fmt.Errorf("read config %s: %w", path, err)
	}
	input = sandbox.Options{
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
		input.MCPServers = make(map[string]sandbox.MCPServer, len(settings.MCP))
		for name, server := range settings.MCP {
			input.MCPServers[name] = sandbox.MCPServer{
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
	if input.Home != "" && !filepath.IsAbs(input.Home) {
		input.Home = filepath.Join(filepath.Dir(path), input.Home)
	}
	return input, nil
}
