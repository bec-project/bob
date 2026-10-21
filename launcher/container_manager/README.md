# Container manager

The launcher runs one OpenCode container, prints its authenticated connection, and
removes the container on shutdown. The home volume persists. Images must already exist
in the selected engine; launches never pull them.

For Go callers, import `bob/container_manager` (package `containermanager`), start with
`DefaultOptions()`, and supply the working directory, provider URL, and any API key.
Call `Manager.Start` and `Manager.Stop` serially. Failed cleanup retains ownership state
for a later `Stop` retry and blocks another start. Cleanup removes only owned containers,
including preparation helpers; a conflicting container name is never removed.

## Configuration

The CLI reads `bob.toml`, or `--config PATH`. See
[bob.example.toml](../../deploy/bob.example.toml) for all settings and defaults.

- **Workspace:** `workspace.workdir` is required and writable. Add mounts with `rw` and
  `ro`; relative paths resolve beside the config, and mounts preserve absolute host paths.
  Read-only and writable mounts cannot overlap, including through symlinks. System
  directories and their aliases, the home volume, and `/tmp` are protected; subdirectories
  such as `/tmp/workspace` are allowed. Custom images must preserve these filesystem assumptions.
- **Provider:** `provider.base_url` must be a container-reachable OpenAI-compatible API.
  `provider.token` overrides `API_KEY`; an explicit empty token clears it.
  Optional `provider.context_limit` sets the model's context window in tokens; OpenCode
  uses it for its context meter and automatic compaction. Optional `provider.output_limit`
  requires `context_limit`, must be smaller, and defaults to min(32768, context_limit/4).
  Both default to 0 (unset). Bob merges the limit into the image's `self-hosted` provider
  through `OPENCODE_CONFIG_CONTENT`, keyed by `provider.model`, so no image rebuild is
  needed; custom images must keep that provider name. Negative values are rejected.
- **MCP:** use named tables such as `[mcp.bec_mcp]`; their keys become OpenCode server
  names. Set `mcp.<name>.url` to an HTTP(S) MCP endpoint outside the container.
  In-container stdio MCP commands are unsupported; TOML `command` entries are rejected.
  Remote OAuth is disabled. HTTP(S) connections do not need a
  published port. Optional `mcp.<name>.port` publishes that server's container TCP port
  on host loopback; it does not start a server. Ports must be unique across servers and
  within 1024–65535, excluding OpenCode's port 4096. A port-only entry just publishes a
  port and does not add an OpenCode MCP connection.
  To launch a host HTTP MCP server automatically, set `host_command` (an argument array)
  alongside `url`. This foreground process runs on the host, outside
  the agent container. Bob waits for its TCP endpoint and stops only the process group
  it created when stopping or on startup failure. An already occupied endpoint is
  rejected. Use `host.docker.internal` or `host.containers.internal` in the container URL;
  bob probes the corresponding host loopback port. Commands inherit bob's launch
  directory, which cannot overlap writable agent mounts, including symlink aliases.
  The executable is also checked, including PATH lookup and symlink aliases. Keep its
  scripts, imported code, and dependencies outside all agent-writable directories.
  On native Linux, the server must listen on a container-reachable interface as well
  as loopback (for bob's readiness probe); a loopback-only listener is insufficient.
  For example, `--host 0.0.0.0` supports both, but exposes the MCP port on every host
  interface, so configure server authentication or restrict inbound access. Docker
  Desktop forwards its host gateway to loopback services.
  Shutdown waits at most 30 seconds for host processes and retains unresolved
  ownership for a cleanup retry.
  Host command output goes to launcher stderr; connection JSON remains on stdout.
  The container allowlist still contains only the provider and MCP endpoint; it does
  not gain direct Redis access or mounts of the host MCP code. Host servers need their
  own dependencies installed on the host.
  Multiple named servers may be configured together. Move old flat `[mcp]` settings
  into `[mcp.bob]` (or another name); flat settings and per-server `command` entries are rejected.
  Connection JSON now returns `mcpPorts`, a map of server names to published host
  addresses, replacing the former single `mcpPort` field. It is omitted when no MCP
  ports are published.
- **Access:** OpenCode serves its API (`serve`) on a generated host port with a generated
  password; bob's GUI is its user interface. The default bind address is `0.0.0.0`;
  use `127.0.0.1` for host-only access. Remote access needs the host's reachable address
  and a trusted network or HTTPS ingress. Treat connection JSON as a credential.

On enforcing SELinux hosts, use pre-labeled mounts or explicitly select
`workspace.mount_label`. Relabeling changes host files, including read-only sources;
avoid system directories and whole home directories. Private labels are unsuitable for
mounts shared with other containers.

## How bob runs

Each user has one bob, started by their first GUI and shared by every client of that user: the
desktop app (one or more windows of it), later BEC widgets. bob has no headless mode; a container runs only
while a client holds it, and for the linger time after the last one leaves.

A per-user broker owns the container. A client finds it with

```sh
bob-manager --config bob.toml [--linger 90s]
```

which makes sure the broker runs, starting a detached one with this bob.toml when none answers,
and prints one JSON line, `{"socket": "<path>"}`. The client then connects to that socket, sends
one JSON line `{"consumer": "<name>", "pid": <pid>, "config": "<absolute bob.toml>"}` and reads
one line back once bob is up: `{"connection": {...}, "config": "...", "consumers": <n>}` or
`{"error": "..."}`. A bob that is already running keeps its own configuration; the reply's
`config` names it; a client that wants another bob.toml first sends `{"stop": true, "reason":
"<text>"}`, which stops bob for every client and ends the broker, and then joins again. The
client holds its lease for as long as it keeps the connection open, so quitting or crashing ends
the lease too. The broker starts bob for the first client and, once the last lease has been gone
for `--linger` (default 90 s, so reopening a GUI keeps bob), stops it the usual way, removing
the container, and exits. A broker that no client joins exits without starting bob. While bob
runs, the broker checks every 5 seconds that its container still runs and its host MCP servers
are alive. Once the container stopped or is gone (an engine restart, an out-of-memory kill), a
host MCP server exited, or the engine has not answered six checks in a row, the broker closes
every lease, cleans up and exits; every client that holds bob first gets `{"stopped": "<why>"}`,
so it learns that bob stopped and the next one starts a new bob. Clients that only use a running
bob, such as background services, should connect to the socket and not start a broker.

The broker is the same executable run with `--broker`, detached in its own session so closing the
client that started it does not reach it. Its socket, lock and log (`broker.log`) live in a
private per-user directory: `$BOB_RUNTIME_DIR`, else `$XDG_RUNTIME_DIR/bob`, else `$TMPDIR/bob`
on macOS, else `/tmp/bob-<uid>`. It must be on a local filesystem and accessible only to its
owner. A lock file ensures one broker per user; a broker that finds the lock taken exits.

The default home volume is `bob-<user name>-home`, so every bob of a user keeps the same sessions
and users of one engine never share them. Set `container.home_volume` to keep an older volume.

## Runtime requirements

Docker uses the caller's UID/GID and supplementary groups on Linux, and UID/GID 1000
elsewhere. Rootless Docker and daemon `userns-remap` are unsupported. Podman uses
`keep-id`; Linux supplementary groups require `crun`. Do not share a home volume
concurrently between launches with different filesystem identities.

Use a current engine with seccomp enabled. The manager selects Docker's built-in profile
(`seccomp=builtin`) or Podman's default policy (`--seccomp-policy default`). Older Docker
clients may interpret `builtin` as a filename. Bootstrap requires active filtering and
NET_ADMIN in the container's own network namespace; unsupported setups fail closed.
Docker is integration-tested; verify Podman and Linux identity mapping on deployment hosts.

## Network policy

Before agent code runs, bootstrap resolves provider/MCP endpoints, pins their addresses
in `/etc/hosts`, and installs default-deny IPv4 and IPv6 outbound rules. It then drops
all capabilities and enables no-new-privileges. Bootstrap failures prevent startup;
no host firewall rules are installed.

Allowed traffic is TCP to configured endpoint IPs and ports (443 for HTTPS or 80 for HTTP
unless specified), local TCP on `127.0.0.1`/`::1`, replies to incoming TCP connections,
and IPv6 neighbour discovery. DNS and other UDP traffic are blocked after bootstrap.
Restart to refresh endpoint addresses after DNS changes.

Rules match IP addresses and ports, so other domains and paths on the same allowed
address and port remain reachable. Upstream servers must enforce path restrictions and
credential scope. Provider tokens are available to the agent and engine administrators.

## Isolation limits

Keep the launcher, its configuration, and other host-executed code outside writable
mounts: the agent can modify scripts or Git hooks that the host may later execute.
Do not mount Docker/Podman or other host-control sockets, even read-only. Supplementary
groups may grant access to mounted sockets. Avoid launching as host root.

Containers do not protect against kernel/runtime vulnerabilities; keep the engine and
runtime patched. Engine administrators can override the policy. Allowed endpoints and
incoming TCP replies remain communication channels, Unix sockets are outside the IP
firewall, and no CPU, memory, or PID limits are configured.

## Development

Run the Go commands from `launcher/` and the firewall tests from the repository root:

```sh
cd launcher
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/container_manager
cd ..
python3 -m unittest discover -s containers -p network_test.py
```

Unit tests need no engine; firewall tests use host Python 3.11+. Docker integration tests
use synthetic HTTP/MCP servers and clean up their own containers and volumes:

```sh
docker build -t localhost/bob ./containers
cd launcher && BOB_CONTAINER_TEST=1 go test -race ./...
```

Rebuild the image after bootstrap changes. See [CI workflows](../../.github/workflows) for
checks and release publishing.

## Release delivery

Main-branch releases run only after CI passes. Semantic release creates the version tag
and GitHub binary assets; delivery downloads those original assets, verifies their
checksums, and uploads them to Gitea before publishing the matching container image.
If delivery fails, rerun the failed workflow jobs. An existing release tag on the same
CI-tested commit is reused; identical files already in Gitea are accepted, while
conflicting files fail delivery. No new release commit or version is needed for a retry.

The checksum manifest must cover every expected binary archive exactly once. Image
publication is serialized; retries may publish an older version's image, but only the
newest published stable GitHub release is promoted to the registry's `latest` tag.

A retry also recovers interrupted GitHub publication for an existing tag on the tested
commit. Missing releases are recreated as drafts; incomplete drafts keep their release
notes and receive a freshly rebuilt, verified asset set before publication. Published
GitHub releases are left untouched, and delivery continues using their original assets.
