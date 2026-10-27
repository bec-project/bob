# Sandbox manager

bob-manager runs OpenCode on the host in a [bubblewrap](https://github.com/containers/bubblewrap)
sandbox, prints its authenticated connection, and stops the sandbox on shutdown. OpenCode's home
folder, with its sessions, persists.

For Go callers, import `bob/container_manager` (package `containermanager`), start with
`DefaultOptions()`, and supply the working directory, provider URL, and any API key.
Call `Manager.Start`, `Manager.Check` and `Manager.Stop` serially. A failed start stops what it
started. The sandbox runs the calling executable with the single argument `--sandbox-helper`;
such a program must then call `RunSandboxHelper` and exit with its result, as bob-manager does.
Cleanup signals only the process groups bob started. A host MCP server that has not exited in
time stays recorded for a later `Stop` retry and blocks another start.

## Configuration

The CLI reads `bob.toml`, or `--config PATH`. See
[bob.example.toml](../../deploy/bob.example.toml) for all settings and defaults.

bob.toml's `[container]` section is now `[sandbox]`, with `bind_address` unchanged. bob-manager
rejects `[container]`, `workspace.mount_label` and `mcp.<name>.port` with an error that says what
to change. `runtime`, `image`, `home_volume` and `name` have no replacement; `sandbox.home` takes
the place of the home volume.

- **Sandbox:** `sandbox.opencode` is the OpenCode executable; a bare name is looked up on the
  PATH (default `opencode`). `sandbox.home` is the folder that keeps OpenCode's home,
  configuration and sessions; it defaults to `$XDG_DATA_HOME/bob/home` (or
  `~/.local/share/bob/home`), and a relative path resolves beside the config. Keep it on a local
  filesystem, not NFS: sessions live in SQLite. `sandbox.bind_address` is described under Access.
- **Workspace:** `workspace.workdir` is required and writable. Add mounts with `rw` and
  `ro`; relative paths resolve beside the config, and mounts keep their absolute host paths in
  the sandbox. Read-only and writable mounts cannot overlap, including through symlinks. Mounts
  cannot overlap the directories the sandbox builds itself (`/usr`, `/etc`, `/bin`, `/sbin`,
  `/lib`, `/lib64`, `/proc`, `/sys`, `/dev`, `/run`, `/bob`) or OpenCode's home
  (`/home/opencode`), and cannot be `/tmp`; subdirectories such as `/tmp/workspace` are allowed.
- **Provider:** `provider.base_url` must be an OpenAI-compatible API that this machine reaches;
  the sandbox reaches it through bob-manager (see [Network policy](#network-policy)).
  `provider.token` overrides `API_KEY`; an explicit empty token clears it.
  Optional `provider.context_limit` sets the model's context window in tokens; OpenCode
  uses it for its context meter and automatic compaction. Optional `provider.output_limit`
  requires `context_limit`, must be smaller, and defaults to min(32768, context_limit/4).
  Both default to 0 (unset). Bob merges the limit into the `self-hosted` provider of its
  OpenCode configuration ([`opencode.json`](opencode.json), built into bob-manager) through
  `OPENCODE_CONFIG_CONTENT`, keyed by `provider.model`. Negative values are rejected.
- **MCP:** use named tables such as `[mcp.bec_mcp]`; their keys become OpenCode server
  names. Set `mcp.<name>.url` to an HTTP(S) MCP endpoint; the sandbox reaches it through
  bob-manager, like the provider. stdio MCP commands in the sandbox are unsupported; TOML
  `command` entries are rejected. Remote OAuth is disabled.
  To launch a host HTTP MCP server automatically, set `host_command` (an argument array)
  alongside `url`. This foreground process runs on the host, outside the sandbox. Its URL must
  use `host.containers.internal` (or `host.docker.internal`), which bob-manager reaches on
  `127.0.0.1`, so the server can listen on loopback only. Bob waits for that TCP endpoint and
  stops only the process group it created when stopping or on startup failure. An already
  occupied endpoint is rejected. Commands inherit bob's launch directory, which cannot overlap
  writable agent mounts, including symlink aliases.
  The executable is also checked, including PATH lookup and symlink aliases. Keep its
  scripts, imported code, and dependencies outside all agent-writable directories.
  Shutdown waits at most 30 seconds for host processes and retains unresolved
  ownership for a cleanup retry.
  Host command output goes to bob-manager's stderr, which for the broker is `broker.log`.
  The sandbox still reaches only the provider and MCP endpoints; it does not gain direct Redis
  access or mounts of the host MCP code. Host servers need their own dependencies installed on
  the host.
  Multiple named servers may be configured together. Move old flat `[mcp]` settings
  into `[mcp.bob]` (or another name); flat settings and per-server `command` entries are rejected.
  Optional `mcp.<name>.plugin` names the GUI plugin for that server's tools (a lowercase name
  such as `bec`; it requires `url`). Connection JSON's `mcpServers` maps every server to
  `{"url", "plugin"}`, where `url` is how this machine reaches it: `host.containers.internal`
  and `host.docker.internal` become `127.0.0.1`.
- **Access:** OpenCode serves its API (`serve`) on `127.0.0.1:4096` in the sandbox with a
  generated password; bob-manager forwards a generated port on `sandbox.bind_address` to it.
  bob's GUI is its user interface. The default bind address is `0.0.0.0`; use `127.0.0.1` for
  host-only access. The connection names `127.0.0.1` when bob listens on every interface, so
  remote access needs the host's reachable address and a trusted network or HTTPS ingress.
  Treat connection JSON as a credential.

## How bob runs

Each user has one bob, started by their first GUI and shared by every client of that user: the
desktop app (one or more windows of it), later BEC widgets. bob has no headless mode; a sandbox
runs only while a client holds it, and for the linger time after the last one leaves.

A per-user broker owns the sandbox. A client finds it with

```sh
bob-manager --config bob.toml [--linger 90s]
```

which makes sure the broker runs, starting a detached one with this bob.toml when none answers,
and prints one JSON line, `{"socket": "<path>"}`. The client then connects to that socket, sends
one JSON line `{"consumer": "<name>", "pid": <pid>, "config": "<absolute bob.toml>"}` and reads
one line back once bob is up: `{"connection": {...}, "config": "...", "consumers": <n>}` or
`{"error": "..."}`. bob is up once OpenCode answers `/global/health` for its generated
password. A bob that is already running keeps its own configuration; the reply's
`config` names it; a client that wants another bob.toml first sends `{"stop": true, "reason":
"<text>"}`, which stops bob for every client and ends the broker, and then joins again. The
client holds its lease for as long as it keeps the connection open, so quitting or crashing ends
the lease too. The broker starts bob for the first client and, once the last lease has been gone
for `--linger` (default 90 s, so reopening a GUI keeps bob), stops it the usual way and exits.
A broker that no client joins exits without starting bob. While bob runs, the broker checks every
5 seconds that its sandbox and its host MCP servers still run. Once the sandbox exited (OpenCode
quit or was killed) or a host MCP server exited, the broker closes every lease, cleans up and
exits; every client that holds bob first gets `{"stopped": "<why>"}`, so it learns that bob
stopped and the next one starts a new bob. Clients that only use a running bob, such as
background services, should connect to the socket and not start a broker.

Stopping bob sends SIGTERM to the sandbox's process group and SIGKILL after 10 seconds, removes
the launch's run directory, and stops its host MCP servers; sessions stay in the home folder. In
the connection JSON, `container` names the launch (`bob-<uid>-<hex>`) and `runtime` is
`bubblewrap`.

The broker is the same executable run with `--broker`, detached in its own session so closing the
client that started it does not reach it. Its socket, lock and log (`broker.log`) live in a
private per-user directory: `$BOB_RUNTIME_DIR`, else `$XDG_RUNTIME_DIR/bob`, else `$TMPDIR/bob`
on macOS, else `/tmp/bob-<uid>`. It must be on a local filesystem and accessible only to its
owner. A lock file ensures one broker per user; a broker that finds the lock taken exits.
OpenCode's output goes to the broker's log as well.

OpenCode's home is `sandbox.home`, by default `$XDG_DATA_HOME/bob/home`, so every bob of a user
keeps the same sessions.

## Runtime requirements

bob's sandbox runs on Linux only. bob-manager and its broker also run on macOS, but starting bob
there fails; run bob in a Linux VM instead. The sandbox needs:

- `bwrap` on bob-manager's PATH, from RHEL 9's BaseOS package `bubblewrap`;
- unprivileged user namespaces, which RHEL 9 enables by default;
- an OpenCode executable, a glibc build since it runs on the host's libraries. bob's packages do
  not include OpenCode: install it and point `sandbox.opencode` at it.

No image, container engine, kernel module, `NET_ADMIN` capability or root access is needed.

## Filesystem

The sandbox has its own user, mount, PID, IPC, UTS and network namespaces (`--unshare-all`) and
its own session (`--new-session`), and ends with bob-manager (`--die-with-parent`). It sees:

- `/usr` and `/etc` read-only from the host, with a generated `/etc/hosts`;
- `/bin`, `/sbin`, `/lib` and `/lib64` as on the host: the same symlinks into `/usr` (as on
  RHEL), or read-only binds;
- a private `/tmp`, `/proc` and `/dev`;
- the sockets folder of the launch's run directory at `/run/bob`, the only host folder it may write
  besides its home and the writable workspace mounts; the run directory is created under
  `$XDG_RUNTIME_DIR` or else the temporary directory, and also holds the generated hosts file and
  OpenCode's configuration, which the sandbox sees read-only as `/etc/hosts` and
  `/bob/opencode.json`;
- `sandbox.home` at `/home/opencode`, OpenCode's `HOME`;
- bob-manager and OpenCode, read-only, in `/bob/bin`;
- the workspace mounts at their own paths, writable or read-only. OpenCode starts in
  `workspace.workdir`.

Nothing else of the host is visible, including the user's home. The sandbox's environment is
built from scratch: `PATH`, `HOME`, `USER`, `LANG`, and what OpenCode's configuration reads (the
provider URL, model and token, the server password, and the MCP servers and model limit).
Secrets go through the environment, never the bwrap command line.

## Network policy

The sandbox has no network. bob-manager carries the connections it needs through Unix sockets
in the run directory's sockets folder, which both sides see:

- **Endpoints:** one tunnel per provider or MCP endpoint (host and port), which bob-manager dials
  on the host for every connection, resolving the name each time. In the sandbox, bob-manager
  itself (`--sandbox-helper`) starts before OpenCode and listens for each endpoint on a loopback
  address (`127.0.0.2`, `127.0.0.3`, …, one per host) that the generated `/etc/hosts` gives the
  endpoint's name, so TLS still checks the real certificate.
- **Clients:** a TCP listener on `sandbox.bind_address`, which bob-manager forwards to the
  helper's `server.sock`; the helper connects it to OpenCode on `127.0.0.1:4096`.

The sandbox cannot bind ports below 1024, so those endpoints move up by 10000 inside: OpenCode
reaches `https://ai.example/api` as `https://ai.example:10443/api`, and the server sees that port
in the `Host` header. An endpoint given as an IP address is reached at its loopback address.
Endpoint URLs cannot use `localhost` or a loopback address, since the sandbox has its own
loopback. `host.containers.internal` and `host.docker.internal` mean this machine: bob-manager
dials `127.0.0.1`, so a host MCP server can listen on `127.0.0.1` only. Two endpoints of one host
whose ports meet in the sandbox (443 and 10443, say) are rejected.

Endpoints are filtered by host and port, so other paths on an allowed host and port remain
reachable. Upstream servers must enforce path restrictions and credential scope. Provider tokens
are available to the agent in its environment.

## Isolation limits

Keep the launcher, its configuration, and other host-executed code outside writable mounts:
the agent can modify scripts or Git hooks that the host may later execute. Do not mount
Docker/Podman or other host-control sockets, even read-only: Unix sockets in a mount stay
usable. Avoid launching as host root.

The agent runs as the user's own UID and groups, so a workspace mount is as readable and
writable to it as to the user. It sees the host's `/usr` and every file in `/etc` the user can
read. There is no seccomp filter and no SELinux confinement yet, and no CPU, memory, or PID
limits. The sandbox relies on the kernel's namespaces and does not protect against kernel or
bubblewrap vulnerabilities; keep both patched. Allowed endpoints remain communication channels.

## Development

Run the Go commands from `launcher/` and the release script tests from the repository root:

```sh
cd launcher
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/container_manager
cd ..
python3 -m unittest discover -s ci -p '*_test.py'
```

Unit tests need no bubblewrap: they put a stand-in `bwrap` first on the PATH and use synthetic
endpoints and host MCP servers. They run on Linux and macOS; the Ctrl-C broker test runs on
Linux only. No test starts a real sandbox, so check changes to the bubblewrap arguments, the
helper or the tunnels on a Linux host with bubblewrap. See
[CI workflows](../../.github/workflows) for checks and release publishing.

## Release delivery

Main-branch releases run only after CI passes. Semantic release creates the version tag
and GitHub binary assets; delivery downloads those original assets, verifies their
checksums, and uploads them to Gitea. The desktop app's bundles follow (see
[released bundles](../../gui/qt/README.md#released-bundles)). Releases carry no container image.
If delivery fails, rerun the failed workflow jobs. An existing release tag on the same
CI-tested commit is reused; identical files already in Gitea are accepted, while
conflicting files fail delivery. No new release commit or version is needed for a retry.

The checksum manifest must cover every expected binary archive exactly once.

A retry also recovers interrupted GitHub publication for an existing tag on the tested
commit. Missing releases are recreated as drafts; incomplete drafts keep their release
notes and receive a freshly rebuilt, verified asset set before publication. Published
GitHub releases are left untouched, and delivery continues using their original assets.
