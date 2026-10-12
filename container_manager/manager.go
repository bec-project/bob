// Package containermanager runs isolated OpenCode backends without a UI dependency.
package containermanager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Manager manages one backend. Calls to its methods must be serialized.
type Manager struct {
	binary string
	name   string
}

type Connection struct {
	URL       string `json:"url"`
	Password  string `json:"password"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"readOnly"`
	Runtime   string `json:"runtime"`
	Workspace string `json:"workspace"`
	MCPPort   string `json:"mcpPort"`
}

func (m *Manager) Start(parent context.Context, input Options) (connection Connection, startErr error) {
	if m.name != "" {
		return connection, errors.New("stop the current assistant before starting another")
	}
	options, err := getResolvedOptions(input)
	if err != nil {
		return connection, err
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if _, err := command(ctx, options.binary, nil, "info"); err != nil {
		return connection, fmt.Errorf("start the %s engine first: %w", options.Runtime, err)
	}
	if _, err := command(ctx, options.binary, nil, "volume", "create", options.HomeVolume); err != nil {
		return connection, err
	}
	options.userConfig, err = prepareUser(ctx, options)
	if err != nil {
		return connection, err
	}
	args, env, err := getLaunchArgs(options)
	if err != nil {
		return connection, err
	}
	// Track the container ID, so a failed create can never remove an existing container.
	id, err := command(ctx, options.binary, env, args...)
	if err != nil {
		return connection, err
	}
	m.binary, m.name = options.binary, id
	defer func() {
		if startErr != nil {
			if err := m.Stop(context.Background()); err != nil {
				startErr = errors.Join(startErr, fmt.Errorf("startup cleanup: %w", err))
			}
		}
	}()
	if _, err := command(ctx, m.binary, nil, "start", m.name); err != nil {
		return connection, err
	}
	connection, err = getConnection(ctx, options)
	if err == nil {
		err = waitReady(ctx, connection)
	}
	if err != nil {
		if parent.Err() != nil {
			return connection, err
		}
		logsCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		logs, _ := command(logsCtx, m.binary, nil, "logs", m.name)
		if options.APIKey != "" {
			logs = strings.ReplaceAll(logs, options.APIKey, "[redacted]")
		}
		return connection, fmt.Errorf("%w\ncontainer logs: %s", err, logs)
	}
	return connection, nil
}

func (m *Manager) Stop(parent context.Context) error {
	if m.name == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if _, err := command(ctx, m.binary, nil, "rm", "--force", m.name); err != nil {
		return err
	}
	m.name, m.binary = "", ""
	return nil
}

func getConnection(ctx context.Context, options resolvedOptions) (Connection, error) {
	address, err := command(ctx, options.binary, nil, "port", options.Name, "4096/tcp")
	if err != nil {
		return Connection{}, err
	}
	connection := Connection{
		URL:       "http://" + strings.Replace(strings.Split(address, "\n")[0], "0.0.0.0:", "127.0.0.1:", 1),
		Password:  options.password,
		Container: options.Name,
		ReadOnly:  options.mounts[0].readOnly,
		Runtime:   options.Runtime,
		Workspace: options.workdir,
	}
	if options.MCPPort != 0 {
		address, err := command(ctx, options.binary, nil, "port", options.Name, fmt.Sprintf("%d/tcp", options.MCPPort))
		if err != nil {
			return connection, err
		}
		connection.MCPPort = strings.Split(address, "\n")[0]
	}
	return connection, nil
}

func waitReady(ctx context.Context, connection Connection) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connection.URL+"/global/health", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("opencode", connection.Password)
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		// Fail promptly when bootstrap or OpenCode exits, rather than waiting 90 seconds.
		state, stateErr := command(ctx, connection.Runtime, nil, "inspect", "--format", "{{.State.Running}}", connection.Container)
		if stateErr == nil && strings.TrimSpace(state) == "false" {
			return fmt.Errorf("OpenCode container exited before becoming ready")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("OpenCode did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
