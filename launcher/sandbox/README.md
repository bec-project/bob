# Sandbox manager

bob-manager runs OpenCode on the host under Anthropic's
[sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime), prints its
authenticated connection, and stops the sandbox on shutdown. OpenCode's home
folder, with its sessions, persists.

For Go callers, import `bob/sandbox`, start with
`DefaultOptions()`, and supply the provider URL and any API key.
Call `Manager.Start`, `Manager.Check` and `Manager.Stop` serially. A failed start stops what it
started. The sandbox runs the calling executable with `--sandbox-helper` followed by OpenCode's
command; such a program must then call `RunSandboxHelper` with that command and exit with its
result, as bob-manager does.
Cleanup signals only the process groups bob started. A host MCP server that has not exited in
time stays recorded for a later `Stop` retry and blocks another start.

## Configuration

The CLI reads `bob.toml`, or `--config PATH`. See
[bob.example.toml](../../deploy/bob.example.toml) for all settings and defaults.

bob-manager rejects any setting it does not know and names it.

- **Sandbox:** `sandbox.opencode` is the OpenCode executable; a bare name is looked up on the
  PATH. By default bob-manager runs the OpenCode installed with it (`../libexec/opencode` beside
  its own executable, as the RPM installs it), else `opencode` on the PATH. `sandbox.runtime` is
  sandbox-runtime's `dist/cli.js`, by default the copy installed with bob-manager. `sandbox.home`
  is the private storage root for sessions, cache, state and generated policy; it defaults to `$XDG_DATA_HOME/bob/home` (or
  `~/.local/share/bob/home`), and a relative path resolves beside the config. Keep it on a local
  filesystem with mode 0700, not NFS: sessions live in SQLite. `sandbox.bind_address` is described under Access.
- **Source access:** direct workspace mounts are unsupported. Remove `[workspace]` from old
  configurations and provide code through the configured BEC MCP source tools. OpenCode's newer
  location loader can execute project plugins even when legacy project-config flags are disabled;
  filesystem isolation and a read-only working directory enforce this MCP-only profile.
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
  bob-manager, like the provider. Only HTTP(S) MCP servers are supported; remote OAuth is
  disabled.
  To launch a host HTTP MCP server automatically, set `host_command` (an argument array)
  alongside `url`. This foreground process runs on the host, outside the sandbox. Its URL must
  name this machine as `localhost` or `127.0.0.1`, so the server can listen on loopback only. Bob waits for that TCP endpoint and
  stops only the process group it created when stopping or on startup failure. An already
  occupied endpoint is rejected. Commands inherit bob's launch directory, which cannot overlap
  `sandbox.home`, including symlink aliases.
  The executable is also checked, including PATH lookup and symlink aliases. Keep its
  scripts, imported code, and dependencies outside all agent-writable directories.
  Shutdown waits at most 30 seconds for host processes and retains unresolved
  ownership for a cleanup retry.
  Host command output goes to bob-manager's stderr, which for the broker is `broker.log`.
  The sandbox still reaches only the provider and MCP endpoints; it does not gain direct Redis
  access or mounts of the host MCP code. Host servers need their own dependencies installed on
  the host.
  Multiple named servers may be configured together.
  Optional `mcp.<name>.plugin` names the GUI plugin for that server's tools (a lowercase name
  such as `bec`; it requires `url`). Connection JSON's `mcpServers` maps every server to
  `{"url", "plugin"}`, where `url` is how this machine reaches it: `localhost` becomes
  `127.0.0.1`.
- **Access:** OpenCode serves its API (`serve`) on `127.0.0.1:4096` in the sandbox with a
  generated password; bob-manager forwards a generated port on `sandbox.bind_address` to it.
  bob's GUI is its user interface. The default bind address is `127.0.0.1`. An explicit remote
  bind needs protected TLS ingress. The connection names `127.0.0.1` when bob listens on every
  interface; remote clients must use the host's reachable address instead.
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
the connection JSON, `name` names the launch (`bob-<uid>-<hex>`) and `runtime` is `srt`.

The broker is the same executable run with `--broker`, detached in its own session so closing the
client that started it does not reach it. Its socket, lock and log (`broker.log`) live in a
private per-user directory: `$BOB_RUNTIME_DIR`, else `$XDG_RUNTIME_DIR/bob`, else `$TMPDIR/bob`
on macOS, else `/tmp/bob-<uid>`. It must be on a local filesystem and accessible only to its
owner. A lock file ensures one broker per user; a broker that finds the lock taken exits.
OpenCode's output goes to the broker's log as well.

OpenCode's persistent storage is under `sandbox.home`, by default `$XDG_DATA_HOME/bob/home`,
so every bob of a user keeps the same sessions.

## Runtime requirements

bob's sandbox runs on Linux only. bob-manager and its broker also run on macOS, but starting bob
there fails; run bob in a Linux VM instead. bob-manager runs OpenCode under Anthropic's
[sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime) (`srt`), which
needs:

- `bwrap` on bob-manager's PATH, from RHEL 9's BaseOS package `bubblewrap`, and unprivileged user
  namespaces, which RHEL 9 enables by default;
- Node.js 22.12 or later on bob-manager's PATH, which srt runs on (RHEL 9 AppStream module
  stream `nodejs:22`: `dnf module enable nodejs:22`);
- `socat` (RHEL 9 AppStream) for srt's proxy bridges, and `bash`;
- srt itself and `rg` (ripgrep), which srt scans the working directory with. The desktop app's
  RPM installs both beside bob-manager (`lib/sandbox-runtime`, `libexec/rg`); elsewhere set
  `sandbox.runtime` to srt's `dist/cli.js` and put `rg` on the PATH;
- OpenCode 1.18.35, a glibc build. The RPM installs it beside bob-manager. Bob checks the selected
  OpenCode release and Node.js version before executing the runtime; upgrading OpenCode requires
  validating its configuration-discovery protections again.
- srt's executable `vendor/seccomp/<architecture>/apply-seccomp`, matching the machine. Bob
  requires this helper explicitly and never accepts srt's missing-helper fallback.

No kernel module, `NET_ADMIN` capability or root access is needed.

## Filesystem

bob-manager uses srt as its README describes, through its command line and settings file and
with its own seccomp filter. It writes srt's settings for each start (`srt.go`) and runs
`node <srt cli.js> --settings <file> -- bob-manager --sandbox-helper opencode serve …`. srt builds a
bubblewrap sandbox without a network namespace of the host's (`--unshare-net`) that ends with
srt (`--die-with-parent`, `--new-session`); with `denyRead: ["/"]` every top-level folder but
`/proc`, `/dev` and `/sys` is an empty tmpfs, and these are re-allowed (`allowRead`):

- OpenCode, bob-manager, `bash` and `socat` (srt runs its bridges with them), and srt's
  `apply-seccomp` helper;
- the library folders `/usr/lib64` and `/usr/lib`: srt binds the paths it is given after
  resolving their symlinks, so single libraries would lose the names the loader asks for;
- `/etc/pki`, `/etc/ssl` and the host's `/etc/hosts` (srt's bridges dial `localhost`);
- only `sandbox.home/.local/share`, `.cache`, and `.local/state` for persistent data, plus
  `sandbox.home/.opencode-policy` for generated read-only configuration and working directory.

Only the three data/cache/state directories are writable. The parent `sandbox.home` is not
exposed, so legacy `.config` and `.opencode` trees are invisible. Data directories cannot be
symlinks into unrelated host storage. Before each launch, bob recreates only the generated
`.opencode-policy` tree, safely replacing poisoned symlinks without following them. Its stable
read-only `home` is both OpenCode's `HOME` and working directory; its configuration is also
read-only. The stable path lets new MCP-only conversations resume after restarting bob.

XDG data/cache/state keep their previous locations. `TMPDIR` and `CLAUDE_CODE_TMPDIR` point at
`.cache/tmp` rather than a shared temporary directory. Direct workspace settings are rejected;
source browsing stays behind MCP, including when resuming an old chat.

Before OpenCode executes, bob's helper requires active filtering and `NoNewPrivs`, then runs
safe syscall probes in a disposable subprocess. Unix and VSOCK sockets, io_uring, new user and
mount namespaces, joining namespaces, and mount operations must be denied with the expected
errors. A filter count alone is not accepted. Missing protection aborts startup and normal
manager cleanup removes its processes and run directory. The kernel keyring, BPF, perf and the
like retain the restrictions that apply to any program of the user.

There is no SELinux confinement: srt's proxies make their Unix sockets on the host as the user's
own type, and a sandbox confined to its own SELinux type could not connect to them.

The environment comes from bob-manager (`PATH`, `HOME`, `USER`, `LANG`, srt's temporary folders,
the CA bundle, and what OpenCode's configuration reads), plus srt's proxy variables and its
`NO_PROXY`.

OpenCode may use the MCP servers' tools and none of its own: bob-manager gives it the permission
rules `"*": "deny"` and `"<server>_*": "allow"` for each MCP server, so OpenCode does not even
offer the model its shell, file, search, web or task tools. Project configuration and unapproved
plugins are disabled, and the read-only policy home prevents discovery of persisted agents,
tools or plugins. Stored provider authentication cannot introduce remote configuration. Before
returning a connection to any client, bob also checks OpenCode's effective configuration and
every agent's native-tool permissions; unexpected policy overrides fail startup. Existing session
permissions are also inspected through the supported global session API. Legacy native-tool grants
are overridden with a final deny-all/MCP-only ruleset, preserving messages and titles; repeated
starts do not append duplicate rules. A session rooted inside writable cache/data/state is refused.
The bounded startup check refuses more than 5000 sessions or a truncated response; use a separate
Bob home for an installation above that limit. The selected launcher, runtime and helper paths,
including intermediate symlinks, must stay outside the
sandbox's persistent storage. The complete SRT installation and its imported npm dependencies
are protected from writable-storage overlap, not only its CLI entrypoint.

## Network policy

srt removes the network and runs an HTTP and a SOCKS5 proxy on the host. Inside, `socat` bridges
them to `localhost:3128` and `localhost:1080`, and `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY`
point there. The proxies use `strictAllowlist` and let through only `network.allowedDomains`, which bob-manager fills with
each endpoint's host and port from bob.toml (`ai.example:443`). Before dialling a name, srt
resolves it and refuses loopback, link-local (including cloud metadata), unspecified, multicast
and broadcast addresses and this host's own addresses.

In bob.toml, `localhost` and `127.0.0.1` mean this machine. srt's `NO_PROXY` sends `localhost`,
`127.0.0.1` and private networks past the proxy into the sandbox's own loopback, where nothing
listens, so OpenCode reaches such an endpoint as
`localhost4.localdomain4:<port>`, which RHEL's `/etc/hosts` maps to 127.0.0.1 alone. bob-manager
allows that name and `127.0.0.1:<port>`; srt's proxy resolves the name on the host and lets the
loopback address through because the literal is on the list (srt's README, "Resolved-address
check"). The MCP server sees that name in the `Host` header, so it must accept it, for the BEC
MCP server with `--allowed-hosts localhost4.localdomain4:<port>`.

srt has no way in: nothing outside its sandbox can reach a server inside. bob-manager listens on
a Unix socket in the run directory and hands that listening socket to srt as standard input,
which srt passes on to the command it runs. The helper accepts on it and pipes each connection to
OpenCode on the sandbox's own `127.0.0.1:4096`; srt's filter refuses creating Unix sockets, not
accepting on an inherited one (its README: "Unix Socket Restrictions"). bob-manager forwards a
TCP listener on `sandbox.bind_address` to that socket. Only the Unix socket goes in: a TCP socket
made outside belongs to the host's network, and code in the sandbox that got hold of it could
disconnect it and connect it to the host's loopback or the LAN, past srt's proxy. A listening Unix
socket cannot be connected anywhere, and the helper refuses any other kind. OpenCode does not
inherit it, and the helper makes itself non-dumpable, so the sandbox's other processes cannot take
it from the helper (`/proc/<pid>/fd`, `pidfd_getfd`) to accept the app's connections themselves.
bob-manager refuses a run directory whose socket path would be too long for Linux (107 bytes).

Endpoints are filtered by host and port, so other paths on an allowed host and port remain
reachable. Upstream servers must enforce path restrictions and credential scope. Provider tokens
are available to the agent in its environment.

## Isolation limits

Keep the launcher, its configuration, host MCP code, and runtime dependencies outside
`sandbox.home`. Host-control sockets are not exposed. Avoid launching as host root.

The agent can call MCP tools only; it has no native shell, file or web tool. The isolated process
still holds bash, socat and libraries required by SRT. It runs as the user's UID under SRT's
filters, with no explicit SELinux domain transition and no automatically configured CPU, memory,
PID or disk limits. The optional systemd envelope below describes a deployment arrangement.

The API credential gives the trusted user control over OpenCode, including configuration and
session permissions; it is not an authorization boundary against a malicious authenticated API
client. Bob's GUI does not send tool-permission overrides. Protect the credential and the API
listener, and do not share them with untrusted clients.

Namespaces do not protect against kernel or bubblewrap vulnerabilities; keep both patched.
Allowed destinations remain communication channels, and endpoint authorization must constrain
what those services permit.

## Development

Run the Go commands from `launcher/` and the release script tests from the repository root:

```sh
cd launcher
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/bob-manager
cd ..
python3 -m unittest discover -s ci -p '*_test.py'
```

Unit tests need no bubblewrap: they put a stand-in `bwrap` first on the PATH and use synthetic
endpoints and host MCP servers. They run on Linux and macOS; the Ctrl-C broker test runs on
Linux only. The opt-in real sandbox tests below exercise the installed runtime on Linux;
ordinary unit tests use stand-ins. See
[CI workflows](../../.github/workflows) for checks and release publishing.

## Real Linux validation

The policy regression uses the actual pinned OpenCode configuration loader and synthetic local
HTTP endpoints. On Linux, cross-compiled test binaries can run it too:

```sh
BOB_OPENCODE_POLICY_TEST=/path/to/opencode \
  go test ./sandbox -run TestOpenCodePolicyWithPinnedExecutable -v
```

From the repository root, run the real SRT integration suite with a built bob-manager and the
pinned OpenCode and SRT installations:

```sh
BOB_SANDBOX_TEST_MANAGER=/path/to/bob-manager \
BOB_SANDBOX_TEST_OPENCODE=/path/to/opencode \
BOB_SANDBOX_TEST_SRT=/path/to/sandbox-runtime/dist/cli.js \
  python3 ci/sandbox_linux_test.py -v
```

These tests use private temporary directories, their own broker and synthetic provider/MCP
endpoints. They do not contact a beamline, real model or production credentials. Ordinary script
test discovery skips them unless the variables are set. Validate the built x86_64 RPM on an
actual RHEL 9 x86_64 target as well; an ARM64 AlmaLinux runtime check does not establish RPM or
architecture compatibility.

## RHEL deployment and resource limits

Install the approved Node stream on deployment hosts before installing the Bob RPM, for example
`dnf module install nodejs:22`. A host already using another stream needs an administrator-reviewed
stream switch; the RPM does not change streams. Install the signed RPM through DNF using only
approved repositories. Runtime startup needs no npm installation or downloads.

Keep the default loopback ingress; workspace settings are unsupported in this profile. Run BEC MCP
under its own confined service and dedicated AI Redis ACL. Bearer-token integration is separate
work: the current unauthenticated example is for synthetic development servers, not the production
MCP authentication policy. Endpoint allowlists do not constrain the APIs or models behind an
allowed gateway; upstream authorization must enforce that scope.

A bounded **foreground broker** is the smallest optional systemd arrangement: its child SRT,
OpenCode, proxies and any host MCP commands inherit the same cgroup, while the GUI stays outside.
For example, place this in `~/.config/systemd/user/bob-broker.service`, after installing the
configuration at `~/.config/bob/bob.toml`:

```ini
[Unit]
Description=Bob sandbox broker

[Service]
Type=exec
WorkingDirectory=/opt/bob-gui
ExecStart=/opt/bob-gui/bin/bob-manager --broker --config %h/.config/bob/bob.toml
Restart=always
RestartSec=1s
KillMode=control-group
TimeoutStopSec=45s
CPUQuota=100%
MemoryHigh=1G
MemoryMax=2G
MemorySwapMax=0
TasksMax=256
LimitNOFILE=4096
UMask=0077
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
```

These are example budgets to measure on the deployment, not automatic Bob defaults. Close clients
and stop any existing unmanaged broker before enabling the unit with `systemctl --user
daemon-reload` and `systemctl --user enable --now bob-broker.service`. Verify the actual process
cgroup and applied limits with `systemctl --user show bob-broker.service -p MainPID -p ControlGroup
-p MemoryMax -p TasksMax -p CPUQuotaPerSecUSec` on a host with the required controllers delegated.

This is an optional operational envelope, not enforced by the GUI launcher. If the unit is stopped
or restarting, a GUI can start an unmanaged broker; its processes do not acquire these limits.
Managed-only launch integration is needed before treating the envelope as an unconditional
availability guarantee. Do not apply namespace restrictions to the outer broker that stop it
from constructing the sandbox.

Foreground-service output goes to the journal instead of `broker.log`; journal retention remains
host policy. CPU/memory/PID limits do not cap the SQLite database, caches or output directories.
Keep `sandbox.home` on local storage and apply site-managed disk quotas and retention. On upgrade,
restart the broker so already-running processes use the new runtime and policy.

## Release delivery

Main-branch releases run only after CI passes. Semantic release creates the version tag
and GitHub binary assets; delivery downloads those original assets, verifies their
checksums, and uploads them to Gitea.
If delivery fails, rerun the failed workflow jobs. An existing release tag on the same
CI-tested commit is reused; identical files already in Gitea are accepted, while
conflicting files fail delivery. No new release commit or version is needed for a retry.

The checksum manifest must cover every expected binary archive exactly once.

A retry also recovers interrupted GitHub publication for an existing tag on the tested
commit. Missing releases are recreated as drafts; incomplete drafts keep their release
notes and receive a freshly rebuilt, verified asset set before publication. Published
GitHub releases are left untouched, and delivery continues using their original assets.
