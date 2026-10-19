package containermanager

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// userSettings maps the caller's filesystem identity into the container.
type userSettings struct {
	User       string
	UserNSMode string
	Groups     []string
}

const launchLabel = "io.bob.launch"

// openCodeProvider is the provider name defined in the image's containers/opencode.json.
const openCodeProvider = "self-hosted"

func seccompArgs(engine string) []string {
	if engine == "docker" {
		// Override daemon defaults, including seccomp=unconfined.
		return []string{"--security-opt", "seccomp=builtin"}
	}
	// Ignore image-provided policies; bootstrap also verifies an active filter.
	return []string{"--seccomp-policy", "default"}
}

// getLaunchArgs contains the complete Docker/Podman container creation command.
// Environment values are prepared separately so the flags stay together.
func getLaunchArgs(options resolvedOptions) ([]string, []string, error) {
	environment, err := getLaunchEnvironment(options)
	if err != nil {
		return nil, nil, err
	}
	args := []string{
		"create",
		"--pull", "never",
		"--name", options.Name,
		"--label", launchLabel + "=" + options.launchID,
		"--user", "0:0",
		"--cap-drop", "ALL",
		"--cap-add", "NET_ADMIN",
		"--cap-add", "SETUID",
		"--cap-add", "SETGID",
		"--cap-add", "SETPCAP",
		"--security-opt", "no-new-privileges:true",
		"--tmpfs", "/tmp:mode=1777",
		"--workdir", options.workdir,
	}
	if options.Runtime == "docker" && runtime.GOOS == "linux" {
		// An empty prepared volume must not inherit the image's UID again.
		args = append(args, "--mount", "type=volume,src="+options.HomeVolume+",dst=/home/opencode,volume-nocopy")
	} else {
		// Without ownership preparation, retain the image's home ownership on copy-up.
		args = append(args, "--volume", options.HomeVolume+":/home/opencode")
	}
	args = append(args, seccompArgs(options.Runtime)...)

	// Runtime-specific identity and host access.
	if options.Runtime == "docker" {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}
	if options.userConfig.UserNSMode != "" {
		args = append(args, "--userns", options.userConfig.UserNSMode)
	}
	for _, group := range options.userConfig.Groups {
		args = append(args, "--group-add", group)
	}

	// Workspace mounts and published ports.
	for _, mount := range options.mounts {
		value := mount.path + ":" + mount.path
		var flags []string
		if mount.readOnly {
			flags = append(flags, "ro")
		}
		switch options.MountLabel {
		case "shared":
			flags = append(flags, "z")
		case "private":
			flags = append(flags, "Z")
		}
		if len(flags) > 0 {
			value += ":" + strings.Join(flags, ",")
		}
		args = append(args, "--volume", value)
	}
	bindAddress := options.BindAddress
	if strings.Contains(bindAddress, ":") {
		bindAddress = "[" + bindAddress + "]"
	}
	args = append(args, "--publish", bindAddress+"::4096/tcp")
	for _, name := range mcpServerNames(options.MCPServers) {
		if port := options.MCPServers[name].Port; port != 0 {
			args = append(args, "--publish", fmt.Sprintf("127.0.0.1::%d/tcp", port))
		}
	}

	// Pass values through the environment to keep tokens out of CLI arguments.
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		args = append(args, "--env", name)
	}
	serverCommand := "serve"
	if options.Web {
		serverCommand = "web"
	}
	args = append(args, options.Image, serverCommand, "--hostname", "0.0.0.0", "--port", "4096")
	return args, environment, nil
}

// The temporary ownership container uses the same home volume as the agent.
func getHomeOwnershipArgs(options resolvedOptions, identity string) []string {
	args := []string{
		"create", "--pull", "never", "--name", "bob-home-" + options.launchID,
		"--label", launchLabel + "=" + options.launchID + "-home",
	}
	args = append(args, seccompArgs(options.Runtime)...)
	return append(args,
		"--user", "0",
		"--entrypoint", "chown",
		"--volume", options.HomeVolume+":/home/opencode",
		options.Image, "-R", "-h", identity, "/home/opencode",
	)
}

func getLaunchEnvironment(options resolvedOptions) ([]string, error) {
	identity := options.userConfig.User
	if identity == "" {
		identity = "1000:1000"
	}
	uid, gid, _ := strings.Cut(identity, ":")
	groups := strings.Join(options.userConfig.Groups, ",")
	if groups == "" {
		groups = gid
	}
	endpoints := []string{options.BaseURL}
	names := mcpServerNames(options.MCPServers)
	for _, name := range names {
		if endpoint := options.MCPServers[name].URL; endpoint != "" {
			endpoints = append(endpoints, endpoint)
		}
	}
	encoded, err := json.Marshal(endpoints)
	if err != nil {
		return nil, err
	}
	environment := []string{
		"BOB_EGRESS_URLS=" + string(encoded), "BOB_UID=" + uid, "BOB_GID=" + gid, "BOB_GROUPS=" + groups,
		"API_KEY=" + options.APIKey, "LLM_BASE_URL=" + options.BaseURL,
		"LLM_MODEL_ID=" + options.Model, "OPENCODE_SERVER_PASSWORD=" + options.password,
	}
	config := map[string]any{}
	mcp := map[string]any{}
	for _, name := range names {
		server := options.MCPServers[name]
		if server.URL != "" {
			mcp[name] = map[string]any{"type": "remote", "url": server.URL, "oauth": false, "enabled": true}
		}
	}
	if len(mcp) > 0 {
		config["mcp"] = mcp
	}
	if contextLimit, outputLimit := options.modelLimit(); contextLimit > 0 {
		// OpenCode deep-merges this into the image configuration, whose model key is the model ID.
		config["provider"] = map[string]any{openCodeProvider: map[string]any{"models": map[string]any{
			options.Model: map[string]any{"limit": map[string]any{"context": contextLimit, "output": outputLimit}},
		}}}
	}
	if len(config) > 0 {
		encoded, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		environment = append(environment, "OPENCODE_CONFIG_CONTENT="+string(encoded))
	}
	return environment, nil
}

// getUserSettings maps the caller's filesystem identity into the container.
func getUserSettings(options resolvedOptions) (userSettings, error) {
	if options.Runtime == "podman" {
		settings := userSettings{UserNSMode: "keep-id:uid=1000,gid=1000"}
		if runtime.GOOS == "linux" {
			settings.Groups = []string{"keep-groups"}
		}
		return settings, nil
	}
	if runtime.GOOS != "linux" {
		return userSettings{}, nil
	}

	identity := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	groups, err := os.Getgroups()
	if err != nil {
		return userSettings{}, err
	}
	var groupsToAdd []string
	for _, group := range groups {
		groupsToAdd = append(groupsToAdd, strconv.Itoa(group))
	}
	return userSettings{User: identity, Groups: groupsToAdd}, nil
}

// Docker host IDs only retain their meaning without daemon user-namespace mapping.
func validateDockerMapping(info string) error {
	var securityOptions []string
	if err := json.Unmarshal([]byte(info), &securityOptions); err != nil {
		return fmt.Errorf("read Docker security options: %w", err)
	}
	for _, option := range securityOptions {
		name := strings.SplitN(option, ",", 2)[0]
		if name == "name=rootless" || name == "name=userns" {
			return fmt.Errorf("Docker rootless and userns-remap modes are unsupported; use rootless Podman with keep-id or Docker without user-namespace remapping")
		}
	}
	return nil
}

func mcpServerNames(servers map[string]MCPServer) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
