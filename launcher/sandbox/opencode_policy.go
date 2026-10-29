package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// openCodePolicy keeps executable configuration outside OpenCode's persistent, writable data.
// The sandbox must expose directory read-only, including home: OpenCode 1.18.35 discovers
// HOME/.opencode even when OPENCODE_DISABLE_PROJECT_CONFIG is set.
type openCodePolicy struct {
	directory string
	home      string
	config    string
}

func prepareOpenCodePolicy(persistentHome string) (openCodePolicy, error) {
	// Keep the working directory stable so persisted sessions retain their project identity.
	// This reserved directory contains generated policy only. RemoveAll removes a poisoned
	// symlink itself, including symlinks inside a directory, without following their targets.
	policy := openCodePolicy{directory: filepath.Join(persistentHome, ".opencode-policy")}
	if err := os.RemoveAll(policy.directory); err != nil {
		return openCodePolicy{}, fmt.Errorf("replace generated OpenCode policy: %w", err)
	}
	policy.home = filepath.Join(policy.directory, "home")
	configDir := filepath.Join(policy.directory, "config", "opencode")
	policy.config = filepath.Join(configDir, "opencode.json")
	for _, directory := range []string{policy.home, configDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return openCodePolicy{}, fmt.Errorf("create OpenCode policy: %w", err)
		}
	}
	if err := os.WriteFile(policy.config, openCodeConfig, 0o600); err != nil {
		return openCodePolicy{}, fmt.Errorf("write OpenCode policy: %w", err)
	}
	// OpenCode synchronously creates this file while loading configuration. It only ignores
	// permission-denied errors, not EROFS from a read-only mount, so prepare it before sandboxing.
	if err := os.WriteFile(filepath.Join(configDir, ".gitignore"), []byte("node_modules\npackage.json\npackage-lock.json\nbun.lock\n.gitignore\n"), 0o600); err != nil {
		return openCodePolicy{}, fmt.Errorf("prepare OpenCode configuration directory: %w", err)
	}
	return policy, nil
}

// environment isolates every automatically discovered configuration directory while retaining
// the existing locations for sessions, cache and state. These flags are supported by the pinned
// OpenCode 1.18.35. In particular, PURE alone does not disable custom agents or tool modules.
func (p openCodePolicy) environment(persistentHome string) []string {
	return []string{
		"HOME=" + p.home,
		"XDG_CONFIG_HOME=" + filepath.Join(p.directory, "config"),
		"XDG_DATA_HOME=" + filepath.Join(persistentHome, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(persistentHome, ".cache"),
		"XDG_STATE_HOME=" + filepath.Join(persistentHome, ".local", "state"),
		"OPENCODE_CONFIG=" + p.config,
		"OPENCODE_CONFIG_DIR=" + filepath.Dir(p.config),
		"OPENCODE_DISABLE_PROJECT_CONFIG=true",
		"OPENCODE_PURE=true",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=true",
		// Do not let auth.json introduce a .well-known/opencode configuration or another provider.
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_MODELS_FETCH=true",
		"OPENCODE_DISABLE_AUTOUPDATE=true",
	}
}

type openCodeEffectiveConfig struct {
	Permission       map[string]any             `json:"permission"`
	MCP              map[string]any             `json:"mcp"`
	Agent            map[string]any             `json:"agent"`
	Mode             map[string]any             `json:"mode"`
	Plugin           []any                      `json:"plugin"`
	Tools            map[string]any             `json:"tools"`
	Provider         map[string]json.RawMessage `json:"provider"`
	EnabledProviders []string                   `json:"enabled_providers"`
	Model            string                     `json:"model"`
}

type openCodeEffectiveAgent struct {
	Name       string                   `json:"name"`
	Permission []openCodePermissionRule `json:"permission"`
}

type openCodePermissionRule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
}

type openCodeSessionPolicy struct {
	ID         string                   `json:"id"`
	Directory  string                   `json:"directory"`
	Permission []openCodePermissionRule `json:"permission"`
}

// verifyOpenCodePolicy checks OpenCode's effective policy before any client receives access.
// OpenCode can merge account configuration from its persistent database after environment
// configuration, so protecting the configuration directories is necessary but not sufficient.
func verifyOpenCodePolicy(parent context.Context, connection Connection, options resolvedOptions) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var config openCodeEffectiveConfig
	if err := readOpenCodePolicy(ctx, client, connection, "/config", &config); err != nil {
		return err
	}
	if err := checkOpenCodeConfig(config, options); err != nil {
		return err
	}
	var agents []openCodeEffectiveAgent
	if err := readOpenCodePolicy(ctx, client, connection, "/agent", &agents); err != nil {
		return err
	}
	if err := checkOpenCodeAgents(agents); err != nil {
		return err
	}
	return normalizeOpenCodeSessionPolicies(ctx, client, connection, options)
}

func readOpenCodePolicy(ctx context.Context, client *http.Client, connection Connection, route string, target any) error {
	_, err := requestOpenCodePolicy(ctx, client, connection, http.MethodGet, route, nil, target, 1<<20)
	return err
}

func requestOpenCodePolicy(ctx context.Context, client *http.Client, connection Connection, method, route string, payload, target any, maxResponse int64) (http.Header, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, errors.New("encode OpenCode policy verification request")
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, connection.URL+route, body)
	if err != nil {
		return nil, errors.New("prepare OpenCode policy verification request")
	}
	request.SetBasicAuth("opencode", connection.Password)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("OpenCode policy verification request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenCode policy verification returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || int64(len(data)) > maxResponse {
		return nil, errors.New("cannot read bounded OpenCode policy response")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return nil, errors.New("OpenCode returned an invalid policy response")
	}
	return response.Header, nil
}

func normalizeOpenCodeSessionPolicies(ctx context.Context, client *http.Client, connection Connection, options resolvedOptions) error {
	// The global endpoint includes other projects and subagents; archived=true also includes
	// archived conversations. Refuse truncation rather than using its timestamp cursor, which
	// can skip sessions sharing a page-boundary timestamp. Bound the work and response size.
	const maxSessions = 5000
	var sessions []openCodeSessionPolicy
	headers, err := requestOpenCodePolicy(ctx, client, connection, http.MethodGet,
		"/experimental/session?archived=true&limit=5001", nil, &sessions, 8<<20)
	if err != nil {
		return err
	}
	if len(sessions) > maxSessions || headers.Get("x-next-cursor") != "" {
		return errors.New("OpenCode has more than 5000 sessions; use a separate bob home before starting safely")
	}
	if sessions == nil {
		return errors.New("OpenCode returned an invalid persisted-session policy list")
	}
	rules := []openCodePermissionRule{{Permission: "*", Pattern: "*", Action: "deny"}}
	for _, name := range mcpServerNames(options.MCPServers) {
		rules = append(rules, openCodePermissionRule{Permission: name + "_*", Pattern: "*", Action: "allow"})
	}
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("verify persisted OpenCode session policies: %w", err)
		}
		if session.ID == "" || strings.ContainsAny(session.ID, "/?#") || session.ID == "." || session.ID == ".." {
			return errors.New("OpenCode returned an invalid persisted-session identifier")
		}
		if err := checkOpenCodeSessionDirectory(session.Directory, options.Home); err != nil {
			return err
		}
		for _, rule := range session.Permission {
			if rule.Permission == "" || rule.Pattern == "" || (rule.Action != "allow" && rule.Action != "ask" && rule.Action != "deny") {
				return errors.New("OpenCode has malformed persisted session permissions; inspect its session data before starting")
			}
		}
		merged := append(append([]openCodePermissionRule(nil), rules...), session.Permission...)
		if checkOpenCodeAgents([]openCodeEffectiveAgent{{Permission: merged}}) == nil {
			continue
		}
		// The supported PATCH endpoint appends permissions; an empty list cannot clear old
		// grants. A final deny-all followed by the configured MCP allowances overrides them
		// without deleting or modifying conversation messages. A later start skips this suffix.
		var updated openCodeSessionPolicy
		if _, err := requestOpenCodePolicy(ctx, client, connection, http.MethodPatch,
			"/session/"+url.PathEscape(session.ID), map[string]any{"permission": rules}, &updated, 1<<20); err != nil {
			return err
		}
		if updated.ID != session.ID || len(updated.Permission) < len(rules) || !reflect.DeepEqual(updated.Permission[len(updated.Permission)-len(rules):], rules) {
			return errors.New("OpenCode did not apply the required persisted-session tool policy")
		}
	}
	return nil
}

func checkOpenCodeSessionDirectory(directory, home string) error {
	if !filepath.IsAbs(directory) {
		return errors.New("OpenCode has an invalid persisted session directory; inspect its session data before starting")
	}
	forms := []string{filepath.Clean(directory)}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		forms = append(forms, resolved)
	} else if !os.IsNotExist(err) {
		return errors.New("cannot verify a persisted OpenCode session directory")
	}
	for _, data := range sandboxDataPaths(home) {
		roots := []string{filepath.Clean(data)}
		if resolved, err := filepath.EvalSymlinks(data); err == nil {
			roots = append(roots, resolved)
		} else if !os.IsNotExist(err) {
			return errors.New("cannot verify the OpenCode persistent data directories")
		}
		for _, root := range roots {
			for _, form := range forms {
				if form == root || strings.HasPrefix(form, root+string(filepath.Separator)) {
					return errors.New("OpenCode has a persisted session inside writable data; use a separate bob home before starting safely")
				}
			}
		}
	}
	return nil
}

func checkOpenCodeConfig(config openCodeEffectiveConfig, options resolvedOptions) error {
	if len(config.Agent) != 0 || len(config.Mode) != 0 || len(config.Plugin) != 0 || len(config.Tools) != 0 {
		return errors.New("OpenCode loaded unapproved agents, plugins or tool configuration")
	}
	urls := []string{options.BaseURL}
	for _, server := range options.MCPServers {
		urls = append(urls, server.URL)
	}
	_, inside, err := getSRTEndpoints(urls)
	if err != nil {
		return errors.New("cannot determine the expected OpenCode endpoint policy")
	}
	sandboxed := options
	sandboxed.BaseURL = inside[options.BaseURL]
	sandboxed.MCPServers = make(map[string]MCPServer, len(options.MCPServers))
	for name, server := range options.MCPServers {
		server.URL = inside[server.URL]
		sandboxed.MCPServers[name] = server
	}
	environment, err := getOpenCodeEnvironment(sandboxed)
	if err != nil {
		return errors.New("cannot determine the expected OpenCode tool policy")
	}
	var expected openCodeEffectiveConfig
	for _, entry := range environment {
		if content, ok := strings.CutPrefix(entry, "OPENCODE_CONFIG_CONTENT="); ok {
			if err := json.Unmarshal([]byte(content), &expected); err != nil {
				return errors.New("cannot decode the expected OpenCode tool policy")
			}
		}
	}
	if !reflect.DeepEqual(config.Permission, expected.Permission) || len(config.MCP) != len(expected.MCP) {
		return errors.New("OpenCode's effective tool policy differs from bob's policy")
	}
	for name, server := range expected.MCP {
		if !reflect.DeepEqual(config.MCP[name], server) {
			return errors.New("OpenCode's effective MCP configuration differs from bob's policy")
		}
	}
	if !reflect.DeepEqual(config.EnabledProviders, []string{openCodeProvider}) || len(config.Provider) != 1 || config.Model != openCodeProvider+"/"+options.Model {
		return errors.New("OpenCode loaded an unapproved model provider")
	}
	var provider struct {
		NPM     string         `json:"npm"`
		Options map[string]any `json:"options"`
	}
	if err := json.Unmarshal(config.Provider[openCodeProvider], &provider); err != nil || provider.NPM != "@ai-sdk/openai-compatible" || !reflect.DeepEqual(provider.Options, map[string]any{"baseURL": sandboxed.BaseURL, "apiKey": options.APIKey}) {
		return errors.New("OpenCode's effective provider configuration differs from bob's policy")
	}
	return nil
}

func checkOpenCodeAgents(agents []openCodeEffectiveAgent) error {
	if len(agents) == 0 {
		return errors.New("OpenCode returned no effective agents")
	}
	for _, agent := range agents {
		// These are the native tool permissions in the pinned OpenCode release. For each one,
		// a final universal denial must override every earlier pattern-specific permission.
		for _, native := range []string{"invalid", "bash", "read", "edit", "glob", "grep", "task", "webfetch", "websearch", "skill", "lsp", "todowrite", "question", "plan_enter", "plan_exit"} {
			denied := false
			for _, rule := range agent.Permission {
				if !openCodePermissionMatches(native, rule.Permission) {
					continue
				}
				if rule.Action != "deny" {
					denied = false
				} else if rule.Pattern == "*" {
					denied = true
				}
			}
			if !denied {
				return errors.New("an effective OpenCode agent permits a native tool")
			}
		}
	}
	return nil
}

// OpenCode's wildcard matcher treats only * and ? as wildcards; unlike filepath.Match, square
// brackets are literal. Permission names have no directory or command-pattern semantics.
func openCodePermissionMatches(name, pattern string) bool {
	if pattern == "*" || pattern == name {
		return true
	}
	pattern = regexp.QuoteMeta(strings.ReplaceAll(pattern, "\\", "/"))
	pattern = strings.ReplaceAll(strings.ReplaceAll(pattern, `\*`, ".*"), `\?`, ".")
	matched, _ := regexp.MatchString("(?s)^"+pattern+"$", name)
	return matched
}
