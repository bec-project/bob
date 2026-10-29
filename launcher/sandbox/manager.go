// Package sandbox runs bob's OpenCode backend in a bubblewrap sandbox, without a UI
// dependency.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Manager manages one backend. Calls to its methods must be serialized.
type Manager struct {
	hostMCPs []*hostMCPProcess
	sandbox  *sandboxProcess
}

type Connection struct {
	URL      string `json:"url"`
	Password string `json:"password"`
	// Name names this launch, so clients can tell one bob from the next.
	Name    string `json:"name"`
	Runtime string `json:"runtime"`
	// MCPServers tells clients where this machine reaches each MCP server that has a URL, and
	// which GUI plugin serves it.
	MCPServers map[string]MCPEndpoint `json:"mcpServers,omitempty"`
}

// MCPEndpoint is one MCP server as clients on this machine reach it.
type MCPEndpoint struct {
	URL    string `json:"url"`
	Plugin string `json:"plugin,omitempty"`
}

// Start starts OpenCode in a sandbox and waits until it answers.
func (m *Manager) Start(parent context.Context, input Options) (Connection, error) {
	if len(m.hostMCPs) > 0 || m.sandbox != nil {
		return Connection{}, errors.New("stop the current assistant before starting another")
	}
	options, err := getResolvedOptions(input)
	if err != nil {
		return Connection{}, err
	}
	return m.startSandbox(parent, options)
}

// ErrStopped marks a Check that found bob gone: its sandbox exited, or one of its host MCP
// servers did.
var ErrStopped = errors.New("bob stopped")

// Check reports whether the started backend still runs: nil while it does, and an error wrapping
// ErrStopped once it does not.
func (m *Manager) Check(context.Context) error {
	if err := m.hostMCPError(); err != nil {
		return fmt.Errorf("%w: %v", ErrStopped, err)
	}
	if m.sandbox == nil {
		return fmt.Errorf("%w: no sandbox was started", ErrStopped)
	}
	return m.sandbox.check()
}

func (m *Manager) Stop(parent context.Context) (stopErr error) {
	defer func() { stopErr = errors.Join(stopErr, m.stopHostMCPs(parent)) }()
	if m.sandbox == nil {
		return nil
	}
	err := m.sandbox.stop(parent)
	m.sandbox = nil
	return err
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
		select {
		case <-ctx.Done():
			return fmt.Errorf("OpenCode did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
