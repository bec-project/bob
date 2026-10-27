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
  PATH. By default bob-manager runs the OpenCode installed with it (`../libexec/opencode` beside
  its own executable, as the RPM installs it), else `opencode` on the PATH. `sandbox.runtime` is
  sandbox-runtime's `dist/cli.js`, by default the copy installed with bob-manager. `sandbox.home`
  is the folder that keeps OpenCode's home,
  configuration and sessions; it defaults to `$XDG_DATA_HOME/bob/home` (or
  `~/.local/share/bob/home`), and a relative path resolves beside the config. Keep it on a local
  filesystem, not NFS: sessions live in SQLite. `sandbox.bind_address` is described under Access.
- **Workspace:** optional. Without `[workspace]`, nothing of the host is mounted and OpenCode
  works in its home; since OpenCode may use only MCP tools, that is enough for bob. When set,
  `workspace.workdir` is writable and OpenCode starts there. Add mounts with `rw` and `ro`; relative paths resolve beside the config, and mounts keep their absolute host paths in
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
there fails; run bob in a Linux VM instead. bob-manager runs OpenCode under Anthropic's
[sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime) (`srt`), which
needs:

- `bwrap` on bob-manager's PATH, from RHEL 9's BaseOS package `bubblewrap`, and unprivileged user
  namespaces, which RHEL 9 enables by default;
- `socat` (RHEL 9 AppStream) for srt's proxy bridges, and `bash`;
- srt itself and `rg` (ripgrep), which srt scans the working directory with. The desktop app's
  RPM installs both beside bob-manager (`lib/sandbox-runtime`, `libexec/rg`); elsewhere set
  `sandbox.runtime` to srt's `dist/cli.js` and put `rg` on the PATH;
- an OpenCode executable, a glibc build. srt runs on the Bun inside it (`BUN_BE_BUN=1`), so no
  Node.js is needed. The RPM installs a pinned OpenCode beside bob-manager.

No image, container engine, kernel module, `NET_ADMIN` capability or root access is needed.

## Filesystem

bob-manager writes srt's settings for each start (`srt.go`) and runs
`opencode <srt cli.js> --settings <file> -- bob-manager --sandbox-helper`. srt builds a
bubblewrap sandbox without a network namespace of the host's (`--unshare-net`) that ends with
srt (`--die-with-parent`, `--new-session`); with `denyRead: ["/"]` every top-level folder but
`/proc`, `/dev` and `/sys` is an empty tmpfs, and these are re-allowed (`allowRead`):

- OpenCode, bob-manager, `bash` and `socat` (srt runs its bridges with them);
- the library folders `/usr/lib64` and `/usr/lib`: srt binds the paths it is given after
  resolving their symlinks, so single libraries would lose the names the loader asks for;
- `/etc/pki`, `/etc/ssl` and the host's `/etc/hosts` (srt's bridges dial `localhost`);
- `sandbox.home`, the run directory's sockets folder and OpenCode's configuration, at their own
  paths, and the workspace mounts, if any.

It may write (`allowWrite`) only `sandbox.home`, the sockets folder and the writable workspace
mounts. OpenCode starts in `workspace.workdir`, or in its home without one, with `HOME` set to
`sandbox.home` and `TMPDIR` inside it.

bob-manager's seccomp filter ([`seccomp.go`](seccomp.go)) refuses new namespaces, mounting, the
kernel keyring, BPF, perf, io_uring, userfaultfd, modules, kexec and the like. srt has no
setting for an extra filter, so bob-manager answers srt's `bwrapPath` itself: as `bwrap`
([`shim.go`](shim.go)) it adds `--add-seccomp-fd` with its filter and becomes the real bwrap.
srt's own Unix-socket filter is off (`allowAllUnixSockets`), because the helper serves the app's
way in on a Unix socket inside.

There is no SELinux confinement: srt's proxies make their Unix sockets on the host as the user's
own type, and a `container_t` sandbox could not connect to them.

The environment comes from bob-manager (`PATH`, `HOME`, `USER`, `LANG`, `TMPDIR`, the CA bundle,
and what OpenCode's configuration reads), plus srt's proxy variables. The helper drops srt's
runner variable, the shim's settings and `NO_PROXY` before it starts OpenCode.

OpenCode may use the MCP servers' tools and none of its own: bob-manager gives it the permission
rules `"*": "deny"` and `"<server>_*": "allow"` for each MCP server, so OpenCode does not even
offer the model its shell, file, search, web or task tools.

## Network policy

srt removes the network and runs an HTTP and a SOCKS5 proxy on the host. Inside, `socat` bridges
them to `localhost:3128` and `localhost:1080`, and `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY`
point there. The proxies let through only `network.allowedDomains`, which bob-manager fills with
each endpoint's host and port from bob.toml (`ai.example:443`). Before dialling a name, srt
resolves it and refuses loopback, link-local (including cloud metadata), unspecified, multicast
and broadcast addresses and this host's own addresses.

`host.containers.internal` and `host.docker.internal` mean this machine: OpenCode reaches such an
MCP server as `127.0.0.1:<port>`, which srt allows as an explicit literal, and the proxy dials it
on the host. srt's `NO_PROXY` would send `127.0.0.1` and private networks past the proxy into the
sandbox's own loopback, so the helper removes it for OpenCode.

Clients reach OpenCode through a TCP listener on `sandbox.bind_address`, which bob-manager
forwards to the helper's `server.sock` in the sockets folder; the helper connects it to OpenCode
on `127.0.0.1:4096`.

Endpoints are filtered by host and port, so other paths on an allowed host and port remain
reachable. Upstream servers must enforce path restrictions and credential scope. Provider tokens
are available to the agent in its environment.

## Isolation limits

Keep the launcher, its configuration, and other host-executed code outside writable mounts:
the agent can modify scripts or Git hooks that the host may later execute. Do not mount
Docker/Podman or other host-control sockets, even read-only: Unix sockets in a mount stay
usable. Avoid launching as host root.

The agent can call the MCP servers' tools and nothing else; it has no shell, file or web tool.
The sandbox itself holds `bash`, `socat` and the system libraries besides OpenCode and
bob-manager, which srt needs. It runs as the user's own UID, so a workspace mount, if any, is as
readable and writable to it as to the user. Its seccomp filter refuses new namespaces and the
other calls listed under [Filesystem](#filesystem); there is no SELinux confinement, and no
CPU, memory, or PID limits. The sandbox relies on the kernel's namespaces and does not protect
against kernel or bubblewrap vulnerabilities; keep both patched. Allowed endpoints remain
communication channels.

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
