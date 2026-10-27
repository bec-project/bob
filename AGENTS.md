# Repository Guidelines — `bob`

`bob` runs OpenCode backends on the host in a bubblewrap sandbox. Prefer focused changes,
follow existing local patterns, and preserve lifecycle cleanup and the sandbox's isolation policy.

This file is an agent-oriented operating manual. Human-facing documentation lives in
[`README.md`](README.md), [`launcher/container_manager/README.md`](launcher/container_manager/README.md), and
[`gui/qt/README.md`](gui/qt/README.md). Configuration examples live in `deploy/bob.example.toml`.

## Core Rules

- Keep sandbox lifecycle, bubblewrap arguments and tunnels in `launcher/container_manager`, bob's
  sandbox manager; keep CLI concerns in `launcher/cmd/container_manager`.
- Calls to one `Manager` must be serialized. Preserve cleanup state until every process bob
  started has exited; a host MCP server that has not been reaped stays recorded for a retry and
  blocks another start.
- Stop only the process groups bob started: the sandbox's and those of its host MCP servers.
  Never stop processes by name or perform broad cleanup.
- The sandbox sees only what it is given. Keep srt's `denyRead: ["/"]` and re-allow nothing
  beyond: OpenCode, bob-manager, `bash` and `socat`, the library folders, `/etc/pki`, `/etc/ssl`
  and `/etc/hosts`, `sandbox.home`, the run directory's sockets folder and OpenCode's
  configuration, and the workspace mounts, if any. It may write only `sandbox.home`, the sockets
  folder and the writable workspace mounts.
- OpenCode may use only MCP tools: keep `"*": "deny"` first in its permission rules, with
  `"<server>_*": "allow"` per MCP server.
- Keep secrets out of bwrap arguments and diagnostics: the provider token and server password go
  through the environment. Keep the sandbox's environment built from scratch; never pass
  bob-manager's own environment through.
- Keep the sandbox without a network of its own. It reaches only the configured provider and MCP
  endpoints, through one tunnel each; do not add tunnels to anything else.
- Preserve mount protections, including read-only/writable overlap checks, symlink aliases, the
  directories the sandbox builds itself, `/home/opencode`, and the reserved `/tmp` destination.
- Connection JSON intentionally contains the generated OpenCode server password; treat it as a
  credential.
- Follow existing patterns before introducing an abstraction. Keep diffs focused and preserve
  unrelated local changes.
- Add regression tests for bug fixes, especially cancellation, a sandbox that exits early, and
  cleanup.
- Do not commit, push, open a PR, or post a review unless explicitly requested.

## First Read

Read the affected implementation and its tests. For unfamiliar areas, start here:

- `launcher/container_manager/options.go` — defaults, validation, mount resolution, and the
  launch's name and password
- `launcher/container_manager/srt.go` — sandbox-runtime's settings, the allowed domains, the
  environment, and startup
- `launcher/container_manager/sandbox.go` — the sandbox process: check and stop, OpenCode lookup,
  and OpenCode's configuration
- `launcher/container_manager/shim.go` — bob-manager as srt's `bwrap`, adding its seccomp filter
- `launcher/container_manager/tunnel.go` — the Unix-socket tunnels and the helper that runs in the
  sandbox (`bob-manager --sandbox-helper`)
- `launcher/container_manager/seccomp.go` — the sandbox's seccomp filter, per architecture
- `launcher/container_manager/manager.go` — the `Manager` API, connection JSON, and readiness
- `launcher/container_manager/host_mcp.go` — host MCP servers: startup, readiness, and
  process-group cleanup
- `launcher/container_manager/runtime.go` — short helper commands with process-group cancellation
- `launcher/container_manager/opencode.json` — OpenCode's base configuration, built into
  bob-manager
- `launcher/cmd/container_manager/config.go` — TOML loading, removed settings, and relative paths
- `launcher/cmd/container_manager/main.go` — arguments, signals, the sandbox helper, and the
  broker-or-locate entry point
- `launcher/cmd/container_manager/broker.go` — the per-user broker that runs one Manager for all of a
  user's clients, and the locate step that starts it
- `launcher/container_manager/sandbox_test.go` and `manager_test.go` — bubblewrap arguments,
  endpoints, tunnels, and the stand-in `bwrap` for startup and cleanup tests

## Repo Layout

- `launcher/` — the Go module (`go.mod`); run Go commands from here
  - `launcher/cmd/container_manager/` — Go executable and CLI/configuration tests
  - `launcher/container_manager/` — reusable Go package, named `containermanager` (bob's sandbox
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
  the errors for removed settings (`[container]`, `workspace.mount_label`, `mcp.<name>.port`);
  update `deploy/bob.example.toml` and the configuration section when public settings change.
- `srt.go`: check the settings (deny-all reads with the listed exceptions, writes, allowed
  domains with ports, `host.containers.internal` as `127.0.0.1`), the environment (srt's runner
  and the shim's settings kept from OpenCode, `NO_PROXY` removed for it), and run a real start on
  Linux: OpenCode must chat and reach its MCP server through srt's proxy. `sandbox.go`: stop
  (process group, SIGTERM then SIGKILL, run directory removed).
- `shim.go`: srt must still find the real bwrap through it, and bob's filter must be active in the
  sandbox (`Seccomp_filters` of OpenCode's process).
- `tunnel.go`: test both directions and half-closes; keep the helper's plan in step with
  `getSandboxEnvironment`, and keep the plan variable out of OpenCode's environment.
- `seccomp.go`: keep both architectures' tables in step; the tests run the program through a small
  BPF interpreter, and on Linux check the numbers against Go's syscall table. Check a change on a
  real kernel: a refused call must fail with `EPERM` inside the sandbox, and OpenCode must still
  start and chat.
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
certificates and the hosts file), holds the provider token in its environment, and runs under a
seccomp filter that refuses new namespaces, mounting and other rarely used kernel calls. There is
no SELinux confinement. Preserve the documented policy and isolation limits
rather than promising stronger guarantees. Host-control sockets remain dangerous even through
read-only mounts.

## Validation

Run the smallest relevant test target first. For lifecycle, shared configuration, or isolation
changes, run the affected suites before finishing. Keep tests independent of execution order.
Go commands run in `launcher/`; the release script tests run from the repository root.

```bash
(cd launcher && go test -race ./container_manager -run TestSandbox)
(cd launcher && go test -race ./cmd/container_manager)
python3 -m unittest discover -s ci -p '*_test.py'
```

Full Go checks, in `launcher/`:

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/container_manager
```

Unit tests should use temporary directories, the stand-in `bwrap`, and synthetic endpoints. Reuse
the existing sandbox, broker, and signal helpers. Do not rely on a real provider, production
credentials, or hardware for tests. The Ctrl-C broker test runs on Linux only.

No test starts a real sandbox. For changes to the bubblewrap arguments, the helper, or the
tunnels, check a real launch on a Linux host with bubblewrap and unprivileged user namespaces,
and say so when you could not. Distinguish unit tests, simulated platform control flow,
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
and configure the workspace, provider URL, any token, and `sandbox.opencode` if OpenCode is not
on the PATH. `bob-manager --config bob.toml` makes sure your broker runs and prints its socket;
bob starts when a client connects there (see
`launcher/container_manager/README.md#how-bob-runs`):

```bash
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
build/bob-manager --config bob.toml
```

Keep the host launcher and other host-executed code outside the agent's writable workspace.

## Platform Notes

Keep host code portable across macOS and Linux: bob-manager and the broker run on both, while the
sandbox runs on Linux only. Subprocess cancellation uses POSIX process groups; Windows is
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
