# Repository Guidelines — `bob`

`bob` runs OpenCode backends on the host in a bubblewrap sandbox. Prefer focused changes,
follow existing local patterns, and preserve lifecycle cleanup and the sandbox's isolation policy.

This file is an agent-oriented operating manual. Human-facing documentation lives in
[`README.md`](README.md), [`launcher/sandbox/README.md`](launcher/sandbox/README.md), and
[`gui/qt/README.md`](gui/qt/README.md). Configuration examples live in `deploy/bob.example.toml`.

## Core Rules

- Keep sandbox lifecycle, bubblewrap arguments and tunnels in `launcher/sandbox`, bob's
  sandbox manager; keep CLI concerns in `launcher/cmd/bob-manager`.
- Calls to one `Manager` must be serialized. Preserve cleanup state until every process bob
  started has exited; a host MCP server that has not been reaped stays recorded for a retry and
  blocks another start.
- Stop only the process groups bob started: the sandbox's and those of its host MCP servers.
  Never stop processes by name or perform broad cleanup.
- The sandbox sees only what it is given. Keep srt's `denyRead: ["/"]` and re-allow nothing
  beyond: OpenCode, bob-manager, `bash` and `socat`, the library folders, srt's `apply-seccomp`,
  `/etc/pki`, `/etc/ssl` and `/etc/hosts`, the three persistent data/cache/state directories,
  and the generated read-only policy tree. Never expose the whole sandbox.home or a workspace.
- Use srt as its README describes: its settings file, its own seccomp filter (never
  `allowAllUnixSockets`), its proxies with its `NO_PROXY`, on Node.js. Add nothing to its
  bubblewrap.
- OpenCode may use only MCP tools: keep `"*": "deny"` first in its permission rules, with
  `"<server>_*": "allow"` per MCP server.
- Keep OpenCode's executable configuration in the read-only generated policy directory, separate
  from persistent sessions/cache. Disable project configuration and unapproved extensions;
  verify effective configuration and agent permissions before returning a connection.
- Require the packaged architecture's SRT helper and verify actual syscall restrictions before
  OpenCode starts. Never accept a missing-helper fallback or a filter count alone.
- Keep secrets out of bwrap arguments and diagnostics: the provider token and server password go
  through the environment. Keep the sandbox's environment built from scratch; never pass
  bob-manager's own environment through.
- Keep the sandbox without a network of its own. It reaches only the configured provider and MCP
  endpoints, through srt's proxies; the app's way in is the only connection into it. Never hand
  the sandbox a TCP or UDP socket made outside: it belongs to the host's network and can be
  reconnected anywhere the host reaches. The way in is a listening Unix socket.
- Reject direct workspace settings. OpenCode's v2 location loader ignores the legacy project
  configuration flags; keep its working directory in the generated read-only policy home.
- Reject symlink aliases from data/cache/state into other host storage. Preserve stored messages
  while replacing legacy session tool grants with the enforced MCP-only suffix.
- Connection JSON intentionally contains the generated OpenCode server password; treat it as a
  credential.
- Follow existing patterns before introducing an abstraction. Keep diffs focused and preserve
  unrelated local changes.
- Add regression tests for bug fixes, especially cancellation, a sandbox that exits early, and
  cleanup.
- Do not commit, push, open a PR, or post a review unless explicitly requested.

## First Read

Read the affected implementation and its tests. For unfamiliar areas, start here:

- `launcher/sandbox/options.go` — defaults, validation, rejected workspace settings, and the
  launch's name and password
- `launcher/sandbox/srt.go` — sandbox-runtime's settings, the allowed domains, the
  environment, and startup
- `launcher/sandbox/sandbox.go` — the sandbox process: check and stop, OpenCode lookup,
  and OpenCode's configuration
- `launcher/sandbox/tunnel.go` — the app's way in and the helper that serves it in the
  sandbox (`bob-manager --sandbox-helper`)
- `launcher/sandbox/manager.go` — the `Manager` API, connection JSON, and readiness
- `launcher/sandbox/host_mcp.go` — host MCP servers: startup, readiness, and
  process-group cleanup
- `launcher/sandbox/runtime.go` — short helper commands with process-group cancellation
- `launcher/sandbox/opencode.json` — OpenCode's base configuration, built into
  bob-manager
- `launcher/cmd/bob-manager/config.go` — TOML loading, removed settings, and relative paths
- `launcher/cmd/bob-manager/main.go` — arguments, signals, the sandbox helper, and the
  broker-or-locate entry point
- `launcher/cmd/bob-manager/broker.go` — the per-user broker that runs one Manager for all of a
  user's clients, and the locate step that starts it
- `launcher/sandbox/sandbox_test.go` and `manager_test.go` — bubblewrap arguments,
  endpoints, tunnels, and the stand-in `bwrap` for startup and cleanup tests

## Repo Layout

- `launcher/` — the Go module (`go.mod`); run Go commands from here
  - `launcher/cmd/bob-manager/` — Go executable and CLI/configuration tests
  - `launcher/sandbox/` — reusable Go package, named `sandbox` (bob's sandbox
    manager), and its tests
- `gui/qt/` — the native desktop app (Qt 6.12, C++ and QML, CMake), which starts and holds bob and
  chats with OpenCode; its C++ backend is in `src/`, its QML module in `imports/Bob/`. Plugins are
  modules under `imports/Bob/Plugins/` with their logic in C++ and their views in QML that imports
  only Qt, `Bob`, `Bob.Backend`'s `Desktop` and the plugin's own types; bob.toml enables one per MCP
  server
- `deploy/` — deployment configuration, including the annotated `bob.example.toml`
- `build/` — ignored local build output
- `.github/workflows/` — reusable CI checks and CI-gated release workflows
- `ci/` — release packaging: bob-manager archives, the Qt app's RPM and DMG, and their Python
  tests

Treat `launcher/go.mod` as the source of truth for Go dependencies and the minimum Go version.
Treat `ruff.toml` as the source of truth for Python linting and formatting settings.

## Local Overlay

If `AGENTS_PERSONAL.md` exists beside this file, read it as an extension of these instructions.
Its machine-specific environment and workflow guidance takes precedence over corresponding generic
guidance here. Keep it local and untracked; do not copy its contents into committed files.

## Common Task Routing

If you change:

- `options.go` or `config.go`: test validation, defaults, token precedence, relative paths, and
  the error that names an unknown setting;
  update `deploy/bob.example.toml` and the configuration section when public settings change.
- `srt.go`: check the settings (deny-all reads with the listed exceptions, writes, allowed
  domains with ports, `localhost` and `127.0.0.1` as `localhost4.localdomain4` plus the
  `127.0.0.1` literal), the environment (srt's temporary folders in `sandbox.home`), and run a
  real start on Linux: OpenCode must chat and reach its MCP server through srt's proxy, and run
  under srt's filter (`Seccomp_filters` of OpenCode's process). `sandbox.go`: stop (process group,
  SIGTERM then SIGKILL, run directory removed).
- `tunnel.go`: test both directions and half-closes through an inherited listener.
- `opencode.json`: keep the `self-hosted` provider name and the variables it reads in step with
  `getOpenCodeEnvironment`.
- `broker.go`: test leases (two clients, one start, none before the first client), teardown after
  the last client and the linger time, a killed client ending its lease, start failures reaching
  every client, cancellation before Stop, the health watch (an exited sandbox ends every lease,
  a few checks that cannot tell do not), a client's stop request and the reason every lease gets,
  and the one-broker lock; run the broker tests repeatedly
  with `-race -count=5`. Keep clients in step with the protocol; the Qt app implements the client
  side in `gui/qt/src/bob/bobsession.cpp`.
- `gui/qt/`: build and run the unit tests (`cmake --build build/qt && (cd build/qt &&
  QT_QPA_PLATFORM=offscreen ctest --output-on-failure)`); keep `imports/Bob/qmldir` and the
  stand-ins in `designer/Bob/Backend` in step with the QML files and the C++ API, so Qt Design Studio
  still opens `bob.qmlproject`. Use only Qt's public API. The MCP client the plugins call through
  (`src/bob/mcpclient.cpp`, `src/gui/plugins.cpp`) must keep calling only servers from the connection of
  the bob the app holds, over `http://` on loopback.
- `manager.go`, `host_mcp.go` or `runtime.go`: test startup failure, cancellation, a sandbox that
  exits before readiness, a host MCP server that exits, cleanup retries, and process-group
  cleanup. Run Go race tests.
- Documentation only: verify paths, commands, examples, and stated runtime limitations. A broad
  test run is unnecessary when executable behavior is unchanged.
- CI or release tooling: run actionlint and the affected checks. Keep publishing in the trusted
  release workflow.

## Lifecycle And Isolation

The launcher resolves configuration, finds `bwrap` and OpenCode, starts host MCP servers, opens
one tunnel per endpoint in a new run directory, starts bwrap in its own process group, and waits
for OpenCode's authenticated health through the client listener. A sandbox that exits early ends
the wait; a failed start stops everything it started.

Cleanup uses a context independent of startup cancellation. Repeated interrupts must not interrupt
cleanup. Stop closes the tunnels, sends SIGTERM to the sandbox's process group and SIGKILL after
10 seconds, waits for bwrap, and removes the run directory; the home folder stays. Host MCP
servers are stopped by process group within 30 seconds; unresolved ones stay recorded for a retry.
Preserve errors for callers through wrapping and `errors.Join`.

Readiness requests must go directly to the client listener, without environment proxies or
redirects. Startup errors include the sandbox's recent output, with the provider token redacted.

The sandbox has no network. It reaches the configured endpoints by host and port through
sandbox-runtime's proxies and does not enforce URL paths. The agent can call only MCP tools, runs
as the user's UID, sees few host files (OpenCode, bash, socat, the library folders, the CA
certificates and the hosts file), holds the provider token in its environment, and runs under
srt's seccomp filter, which refuses Unix sockets, io_uring, new namespaces and mounting. There is
no SELinux confinement. Preserve the documented policy and isolation limits
rather than promising stronger guarantees. Host-control sockets remain dangerous even through
read-only mounts.

## Validation

Run the smallest relevant test target first. For lifecycle, shared configuration, or isolation
changes, run the affected suites before finishing. Keep tests independent of execution order.
Go commands run in `launcher/`; the release script tests run from the repository root.

```bash
(cd launcher && go test -race ./sandbox -run TestSandbox)
(cd launcher && go test -race ./cmd/bob-manager)
python3 -m unittest discover -s ci -p '*_test.py'
```

Full Go checks, in `launcher/`:

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/bob-manager
```

Unit tests should use temporary directories, the stand-in `bwrap`, and synthetic endpoints. Reuse
the existing sandbox, broker, and signal helpers. Do not rely on a real provider, production
credentials, or hardware for tests. The Ctrl-C broker test runs on Linux only.

For isolation changes, run the opt-in real Linux suite in `ci/sandbox_linux_test.py` and the
pinned executable policy regression; see the sandbox manager's README. Say so when the required
Linux host or binaries are unavailable. Distinguish unit tests, simulated platform control flow,
cross-compilation, and actual deployment checks.

## Style And Change Hygiene

- Run `gofmt` on changed Go files; document new exported APIs with Go doc comments.
- Use Python 3.11-compatible syntax, four-space indentation, and a 100-character line limit.
- Prefer f-strings and `pathlib` in new Python code where appropriate.
- Use argument arrays for subprocesses; do not build shell commands from configuration values.
- Keep the release scripts' Python dependencies in the standard library unless there is a
  concrete need.
- Avoid formatting and import-order churn in untouched files.

```bash
ruff format --config ruff.toml ci
ruff check --config ruff.toml ci
```

## Development Environment

Requires Go 1.25+ and host Python 3.11+. Running bob needs Linux with `bwrap` on the PATH,
unprivileged user namespaces, and an OpenCode executable; unit tests need none of these. Use an
existing suitable environment; keep machine paths and environment activation instructions out of
committed repository guidance.

If the default Go cache is unavailable in a sandbox, set `GOCACHE` to a writable temporary
directory for that command. Do not change global tooling configuration to run one task.

For a local launch, build bob-manager, copy `deploy/bob.example.toml` to the ignored `bob.toml`,
and configure the provider URL, any token, and `sandbox.opencode` if OpenCode is not
on the PATH. `bob-manager --config bob.toml` makes sure your broker runs and prints its socket;
bob starts when a client connects there (see
`launcher/sandbox/README.md#how-bob-runs`):

```bash
go -C launcher build -o ../build/bob-manager ./cmd/bob-manager
build/bob-manager --config bob.toml
```

Keep the host launcher and other host-executed code outside sandbox.home and all agent-writable storage.

## Platform Notes

Keep host code portable across macOS and Linux: bob-manager, the broker and the sandbox run on
both, macOS for development only (its sandbox is weaker; see the sandbox README). Subprocess cancellation uses POSIX process groups; Windows is
unsupported by the current implementation.

bubblewrap needs unprivileged user namespaces, which RHEL 9 enables by default. Verify the sandbox
on deployment hosts rather than inferring it from argument tests. Keep `sandbox.home` off NFS:
OpenCode keeps its sessions in SQLite.

## Commit And PR Notes

- Branch from `main` for new work; continue an existing PR on its current branch.
- Use Conventional Commit titles. A breaking-change commit message requires explicit user approval
  before committing: do not add `!` or a `BREAKING CHANGE:` footer without that approval.
- Permission to commit or push does not authorize a breaking-change marker. If a change requires
  one, explain the compatibility impact and obtain explicit approval before committing it.
- Lead descriptions with the concrete problem and resulting behavior.
- Explain configuration/default changes and compatibility implications.
- Give exact validation commands and results, including platform checks you could not run, such
  as a real sandbox on Linux.
- Return reviews in chat unless posting externally is explicitly requested; distinguish introduced,
  exposed, and pre-existing findings.
