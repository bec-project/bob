// Package containermanager runs isolated OpenCode backends without a UI dependency.
package containermanager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Manager manages one backend. Calls to its methods must be serialized.
type Manager struct {
	binary          string
	name            string
	launchID        string
	createUncertain bool
	home            *Manager
	hostMCPs        []*hostMCPProcess
}

type Connection struct {
	URL       string `json:"url"`
	Password  string `json:"password"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"readOnly"`
	Runtime   string `json:"runtime"`
	Workspace string `json:"workspace"`
	// MCPPorts maps MCP server names to their published host addresses.
	MCPPorts map[string]string `json:"mcpPorts,omitempty"`
}

func (m *Manager) Start(parent context.Context, input Options) (connection Connection, startErr error) {
	if m.name != "" || m.launchID != "" || m.home != nil || len(m.hostMCPs) > 0 {
		return connection, errors.New("stop the current assistant before starting another")
	}
	options, err := getResolvedOptions(input)
	if err != nil {
		return connection, err
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	infoArgs := []string{"info"}
	if options.Runtime == "docker" {
		infoArgs = append(infoArgs, "--format", "{{json .SecurityOptions}}")
	}
	info, err := command(ctx, options.binary, nil, infoArgs...)
	if err != nil {
		return connection, fmt.Errorf("start the %s engine first: %w", options.Runtime, err)
	}
	if options.Runtime == "docker" {
		if err := validateDockerMapping(info); err != nil {
			return connection, err
		}
	}
	if input.Name != "" {
		names, err := command(ctx, options.binary, nil, "ps", "--all", "--filter", "name="+options.Name, "--format", "{{.Names}}")
		if err != nil {
			return connection, err
		}
		for _, name := range strings.Fields(names) {
			if name == options.Name {
				return connection, fmt.Errorf("container name %s is already in use", options.Name)
			}
		}
	}
	if _, err := command(ctx, options.binary, nil, "image", "inspect", options.Image); err != nil {
		return connection, fmt.Errorf("make the agent image available in the engine before launching: %w", err)
	}
	if _, err := command(ctx, options.binary, nil, "volume", "create", options.HomeVolume); err != nil {
		return connection, err
	}
	options.userConfig, err = getUserSettings(options)
	if err != nil {
		return connection, err
	}
	args, env, err := getLaunchArgs(options)
	if err != nil {
		return connection, err
	}
	defer func() {
		if startErr != nil {
			if err := m.Stop(context.Background()); err != nil {
				startErr = errors.Join(startErr, fmt.Errorf("startup cleanup: %w", err))
			}
		}
	}()
	if err := m.startHostMCPs(ctx, options.MCPServers); err != nil {
		return connection, err
	}
	if options.Runtime == "docker" && runtime.GOOS == "linux" {
		if err := m.prepareHome(ctx, options); err != nil {
			return connection, err
		}
	}
	if err := m.createContainer(ctx, options.binary, options.launchID, env, args...); err != nil {
		return connection, err
	}
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
		logs, _ := commandLogs(logsCtx, m.binary, "logs", m.name)
		if options.APIKey != "" {
			logs = strings.ReplaceAll(logs, options.APIKey, "[redacted]")
		}
		return connection, fmt.Errorf("%w\ncontainer logs: %s", err, logs)
	}
	if err := m.hostMCPError(); err != nil {
		return connection, err
	}
	return connection, nil
}

func (m *Manager) createContainer(ctx context.Context, binary, launchID string, env []string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Every failure after dispatch can lose an acknowledgement, including
	// transport errors. Track ownership before issuing the engine command.
	m.binary, m.launchID, m.createUncertain = binary, launchID, true
	id, err := command(ctx, binary, env, args...)
	if err != nil {
		var failure *commandFailure
		if errors.As(err, &failure) && failure.creationSettled {
			m.createUncertain = false
		}
		return err
	}
	if id == "" {
		return errors.New("container engine returned no container ID")
	}
	m.name, m.createUncertain = id, false
	return nil
}

func (m *Manager) prepareHome(ctx context.Context, options resolvedOptions) error {
	m.home = &Manager{}
	if err := m.home.createContainer(ctx, options.binary, options.launchID+"-home", nil,
		getHomeOwnershipArgs(options, options.userConfig.User)...); err != nil {
		return err
	}
	if _, err := command(ctx, options.binary, nil, "start", "--attach", m.home.name); err != nil {
		return err
	}
	// start --attach does not reliably propagate the container's exit status.
	code, err := command(ctx, options.binary, nil, "inspect", "--format", "{{.State.ExitCode}}", m.home.name)
	if err != nil {
		return err
	}
	if code != "0" {
		return fmt.Errorf("home ownership preparation exited with status %s", code)
	}
	if err := m.home.Stop(context.Background()); err != nil {
		return err
	}
	m.home = nil
	return nil
}

func (m *Manager) Stop(parent context.Context) (stopErr error) {
	defer func() { stopErr = errors.Join(stopErr, m.stopHostMCPs(parent)) }()
	if m.home != nil {
		if err := m.home.Stop(parent); err != nil {
			return fmt.Errorf("home ownership cleanup: %w", err)
		}
		m.home = nil
	}
	if m.name == "" && m.launchID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	ids := []string{m.name}
	if m.name == "" {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			var err error
			ids, err = m.ownedContainers(ctx)
			if err != nil {
				return err
			}
			if len(ids) > 0 || !m.createUncertain {
				break
			}
			// Killing the CLI does not settle the daemon's create request.
			// An empty snapshot must not discard ownership of a late container.
			select {
			case <-ctx.Done():
				return fmt.Errorf("creation outcome still unknown for launch %s: %w", m.launchID, ctx.Err())
			case <-ticker.C:
			}
		}
	}
	if len(ids) > 0 {
		m.createUncertain = false
		if _, err := command(ctx, m.binary, nil, append([]string{"rm", "--force"}, ids...)...); err != nil {
			// The daemon may have removed the container before the CLI failed.
			// Clear state only when an independent query confirms its absence.
			remaining, lookupErr := m.ownedContainers(ctx)
			if lookupErr != nil {
				return errors.Join(err, fmt.Errorf("verify removal: %w", lookupErr))
			}
			if len(remaining) > 0 {
				return err
			}
		}
	}
	m.name, m.binary, m.launchID = "", "", ""
	m.createUncertain = false
	return nil
}

func (m *Manager) ownedContainers(ctx context.Context) ([]string, error) {
	filter := "label=" + launchLabel + "=" + m.launchID
	if m.launchID == "" {
		filter = "id=" + m.name
	}
	output, err := command(ctx, m.binary, nil, "ps", "--all", "--filter", filter, "--format", "{{.ID}}")
	return strings.Fields(output), err
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
	for _, name := range mcpServerNames(options.MCPServers) {
		port := options.MCPServers[name].Port
		if port == 0 {
			continue
		}
		address, err := command(ctx, options.binary, nil, "port", options.Name, fmt.Sprintf("%d/tcp", port))
		if err != nil {
			return connection, err
		}
		if connection.MCPPorts == nil {
			connection.MCPPorts = map[string]string{}
		}
		connection.MCPPorts[name] = strings.Split(address, "\n")[0]
	}
	return connection, nil
}

func waitReady(ctx context.Context, connection Connection) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connection.URL+"/global/health", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("opencode", connection.Password)
	transport := &http.Transport{}
	if defaults, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaults.Clone()
	}
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:       2 * time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
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
