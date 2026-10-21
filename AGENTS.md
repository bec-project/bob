# Repository Guidelines — `bob`

`bob` launches isolated OpenCode backends through Docker or Podman. Prefer focused changes,
follow existing local patterns, and preserve lifecycle cleanup and the container's isolation policy.

This file is an agent-oriented operating manual. Human-facing documentation lives in
[`README.md`](README.md), [`launcher/container_manager/README.md`](launcher/container_manager/README.md), and
[`containers/README.md`](containers/README.md). Configuration examples live in `deploy/bob.example.toml`.

## Core Rules

- Keep container lifecycle and runtime arguments in `launcher/container_manager`; keep CLI concerns in
  `launcher/cmd/container_manager`.
- Calls to one `Manager` must be serialized. Preserve cleanup state until ownership and removal
  are settled; an empty lookup does not settle an uncertain create request.
- Track agent containers and preparation helpers by ownership labels. Never remove an unrelated
  container by a configured name or perform broad engine cleanup.
- Bootstrap must fail closed. Agent code must not run before active seccomp filtering, IPv4/IPv6
  firewall setup, and permanent capability dropping are established.
- Install firewall rules only inside the container's network namespace, never on the host.
- Preserve mount protections, including read-only/writable overlap checks, symlink aliases,
  system-directory aliases, the home volume, and the reserved `/tmp` destination.
- Keep provider tokens out of command arguments and diagnostics. Connection JSON intentionally
  contains the generated OpenCode server password; treat it as a credential.
- Follow existing patterns before introducing an abstraction. Keep diffs focused and preserve
  unrelated local changes.
- Add regression tests for bug fixes, especially cancellation, lost acknowledgements, and cleanup.
- Do not commit, push, open a PR, or post a review unless explicitly requested.

## First Read

Read the affected implementation and its tests. For unfamiliar areas, start here:

- `launcher/container_manager/options.go` — defaults, validation, mount resolution, and ownership identity
- `launcher/container_manager/launch.go` — engine arguments, environment, and filesystem identity mapping
- `launcher/container_manager/manager.go` — startup, helper lifecycle, cleanup, and readiness
- `launcher/container_manager/runtime.go` — subprocess execution, cancellation, and failure classification
- `launcher/cmd/container_manager/config.go` — TOML loading and relative paths
- `launcher/cmd/container_manager/main.go` — arguments, signals, and the broker-or-locate entry point
- `launcher/cmd/container_manager/broker.go` — the per-user broker that runs one Manager for all of a
  user's clients, and the locate step that starts it
- `containers/network.py` and `containers/entrypoint.sh` — firewall bootstrap and privilege dropping
- `containers/Dockerfile` and `containers/opencode.json` — image and OpenCode configuration
- `launcher/container_manager/lifecycle_test.go` — fake engine and cleanup regression tests

## Repo Layout

- `launcher/` — the Go module (`go.mod`); run Go commands from here
  - `launcher/cmd/container_manager/` — Go executable and CLI/configuration tests
  - `launcher/container_manager/` — reusable Go package, named `containermanager`, and its tests
- `containers/` — what bob ships as containers: the image build context, Python firewall bootstrap,
  shell entrypoint, and network tests
- `deploy/` — deployment configuration, including the annotated `bob.example.toml`
- `build/` — ignored local build output
- `.github/workflows/` — reusable CI checks and CI-gated release workflows
- `ci/` — release binary packaging

Treat `launcher/go.mod` as the source of truth for Go dependencies and the minimum Go version.
Treat `ruff.toml` as the source of truth for Python linting and formatting settings.

## Local Overlay

If `AGENTS_PERSONAL.md` exists beside this file, read it as an extension of these instructions.
Its machine-specific environment and workflow guidance takes precedence over corresponding generic
guidance here. Keep it local and untracked; do not copy its contents into committed files.

## Common Task Routing

If you change:

- `options.go` or `config.go`: test validation, defaults, token precedence, and relative paths;
  update `deploy/bob.example.toml` and the configuration table when public settings change.
- `launch.go`: check Docker and Podman argument generation, identities, security options, mount
  flags, and helper commands. Preserve values without shell interpolation.
- `broker.go`: test leases (two clients, one start, none before the first client), teardown after
  the last client and the linger time, a killed client ending its lease, start failures reaching
  every client, cancellation before Stop, the health watch (a stopped container ends every lease,
  brief engine failures do not), a client's stop request and the reason every lease gets, and the
  one-broker lock; run the broker tests repeatedly
  with `-race -count=5`. Keep clients in step with the protocol.
- `manager.go` or `runtime.go`: test startup failure, cancellation, delayed creation, cleanup
  retries, helper exit status, and lost removal acknowledgements. Run Go race tests.
- `network.py`: run the network unit tests; check both address families, mapped IPv4 addresses,
  DNS pinning, loopback handling, and failures before agent execution.
- `entrypoint.sh`, `Dockerfile`, or `opencode.json`: rebuild the image before real-engine
  validation. Check bootstrap identity, capability dropping, provider configuration, and MCP setup.
- Documentation only: verify paths, commands, examples, and stated runtime limitations. A broad
  test run is unnecessary when executable behavior is unchanged.
- CI or release tooling: run actionlint and the affected checks. Keep publishing in the trusted
  release workflow.

## Lifecycle And Isolation

The launcher resolves configuration, checks the engine and local image, prepares the home volume,
creates and starts the agent container, discovers published ports, and waits for authenticated
health.
Images must already exist in the selected engine; creation uses `--pull never`.

Engine CLI termination does not necessarily cancel a daemon request. Record ownership before
dispatch. Keep transport errors, proxy/server timeouts, and unknown responses uncertain. Only settle
a rejection when evidence establishes that creation cannot complete; do not generalize from an
error prefix alone. Preserve errors for callers through wrapping and `errors.Join`.

Cleanup uses a context independent of startup cancellation. Repeated interrupts must not interrupt
cleanup. Successful stop clears state; unresolved cleanup retains enough state for a retry and
blocks another start. Check preparation helpers' exit status and clean them up like agent
containers.

Readiness requests must go directly to the published container endpoint, without environment proxies
or redirects. Structured engine responses use stdout only; container diagnostics need both streams.

The firewall allows configured server IPs and TCP ports, local TCP services, replies to incoming TCP
connections, and IPv6 neighbour discovery. It does not enforce URL paths or distinguish domains on
the same IP. Preserve the documented policy and isolation limits rather than promising stronger
guarantees. Host-control sockets remain dangerous even through read-only mounts.

## Validation

Run the smallest relevant test target first. For lifecycle, shared configuration, or isolation
changes, run the affected suites before finishing. Keep tests independent of execution order.
Go commands run in `launcher/`; the firewall tests run from the repository root.

```bash
(cd launcher && go test -race ./container_manager -run TestCancelledCreate)
(cd launcher && go test -race ./cmd/container_manager)
python3 -m unittest discover -s containers -p network_test.py
```

Full Go checks, in `launcher/`:

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o ../build/bob-manager ./cmd/container_manager
```

Unit tests should use temporary directories, fake engines, and synthetic endpoints. Reuse the
existing lifecycle and signal helpers; model helper and agent containers separately. Do not rely on
a real provider, production credentials, or hardware for tests.

Real-engine integration requires Docker and a freshly built image:

```bash
docker build -t localhost/bob ./containers
(cd launcher && BOB_CONTAINER_TEST=1 go test -race ./...)
```

Integration tests must clean up only their own containers and test volumes. Ordinary manager cleanup
preserves the home volume. Report skipped integration tests and distinguish unit tests, simulated
platform control flow, cross-compilation, and actual deployment checks.

## Style And Change Hygiene

- Run `gofmt` on changed Go files; document new exported APIs with Go doc comments.
- Use Python 3.11-compatible syntax, four-space indentation, and a 100-character line limit.
- Prefer f-strings and `pathlib` in new Python code where appropriate.
- Use argument arrays for subprocesses; do not build shell commands from configuration values.
- Keep Python bootstrap dependencies in the standard library unless there is a concrete need.
- Avoid formatting and import-order churn in untouched files.

```bash
ruff format containers/network.py containers/network_test.py
ruff check containers/network.py containers/network_test.py
```

## Development Environment

Requires Go 1.25+, host Python 3.11+, and a current Docker or Podman engine for integration work.
Unit tests normally need no running engine. Use an existing suitable environment; keep machine paths
and environment activation instructions out of committed repository guidance.

If the default Go cache is unavailable in a sandbox, set `GOCACHE` to a writable temporary
directory for that command. Do not change global tooling configuration to run one task.

For a local launch, build the image, copy `deploy/bob.example.toml` to the ignored `bob.toml`, and
configure
the workspace, engine, provider URL, and any token. `bob-manager --config bob.toml` makes sure your
broker runs and prints its socket; bob starts when a client connects there (see
`launcher/container_manager/README.md#how-bob-runs`):

```bash
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
build/bob-manager --config bob.toml
```

Keep the host launcher and other host-executed code outside the agent's writable workspace.

## Platform Notes

Keep host code portable across macOS and Linux. Subprocess cancellation uses POSIX process groups;
Windows is unsupported by the current implementation.

Docker maps the host UID/GID on Linux and uses UID/GID 1000 on other hosts. Rootless Docker and
daemon `userns-remap` are rejected. Podman uses `keep-id`; Linux supplementary groups require
`crun`.
Verify identity mapping on deployment hosts rather than inferring it from argument tests.

On enforcing SELinux hosts, use pre-labeled mounts or explicitly select `workspace.mount_label`.
Relabeling changes host files, including read-only sources; do not silently enable it for users.
Do not share a home volume concurrently between launches using different filesystem identities.

## Commit And PR Notes

- Branch from `main` for new work; continue an existing PR on its current branch.
- Use Conventional Commit titles. A breaking-change commit message requires explicit user approval
  before committing: do not add `!` or a `BREAKING CHANGE:` footer without that approval.
- Permission to commit or push does not authorize a breaking-change marker. If a change requires
  one, explain the compatibility impact and obtain explicit approval before committing it.
- Lead descriptions with the concrete problem and resulting behavior.
- Explain configuration/default changes, image rebuild requirements, and compatibility implications.
- Give exact validation commands and results, including unavailable engines or platform checks.
- Return reviews in chat unless posting externally is explicitly requested; distinguish introduced,
  exposed, and pre-existing findings.
