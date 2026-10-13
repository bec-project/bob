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
	Container struct {
		Web         bool   `toml:"web"`
		Runtime     string `toml:"runtime"`
		Image       string `toml:"image"`
		HomeVolume  string `toml:"home_volume"`
		Name        string `toml:"name"`
		BindAddress string `toml:"bind_address"`
	} `toml:"container"`
	Workspace struct {
		Workdir    string   `toml:"workdir"`
		RW         []string `toml:"rw"`
		RO         []string `toml:"ro"`
		MountLabel string   `toml:"mount_label"`
	} `toml:"workspace"`
	Provider struct {
		BaseURL string  `toml:"base_url"`
		Model   string  `toml:"model"`
		Token   *string `toml:"token"`
	} `toml:"provider"`
	MCP map[string]struct {
		Port        int      `toml:"port"`
		URL         string   `toml:"url"`
		HostCommand []string `toml:"host_command"`
		// Decode only to explain the removed setting; never forward or execute it.
		UnsupportedCommand any `toml:"command"`
	} `toml:"mcp"`
}

func loadConfig(path string) (containermanager.Options, error) {
	input := containermanager.DefaultOptions()
	var settings config
	settings.Container.Runtime = input.Runtime
	settings.Container.Image = input.Image
	settings.Container.HomeVolume = input.HomeVolume
	settings.Container.BindAddress = input.BindAddress
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
	input = containermanager.Options{
		Runtime:     settings.Container.Runtime,
		Web:         settings.Container.Web,
		Image:       settings.Container.Image,
		HomeVolume:  settings.Container.HomeVolume,
		Name:        settings.Container.Name,
		Workdir:     settings.Workspace.Workdir,
		ReadWrite:   settings.Workspace.RW,
		ReadOnly:    settings.Workspace.RO,
		MountLabel:  settings.Workspace.MountLabel,
		BaseURL:     settings.Provider.BaseURL,
		Model:       settings.Provider.Model,
		BindAddress: settings.Container.BindAddress,
		APIKey:      os.Getenv("API_KEY"),
	}
	if len(settings.MCP) > 0 {
		input.MCPServers = make(map[string]containermanager.MCPServer, len(settings.MCP))
		for name, server := range settings.MCP {
			if server.UnsupportedCommand != nil {
				return input, fmt.Errorf("mcp.%s.command is unsupported; use url with host_command for host startup", name)
			}
			input.MCPServers[name] = containermanager.MCPServer{
				Port: server.Port, URL: server.URL,
				HostCommand: server.HostCommand,
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
	if input.Workdir == "" {
		return input, errors.New("config requires workspace.workdir")
	}

	// Resolve paths beside the config so it works from any launching directory.
	if !filepath.IsAbs(input.Workdir) {
		input.Workdir = filepath.Join(filepath.Dir(path), input.Workdir)
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
