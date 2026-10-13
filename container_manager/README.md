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
[bob.example.toml](../bob.example.toml) for all settings and defaults.

- **Workspace:** `workspace.workdir` is required and writable. Add mounts with `rw` and
  `ro`; relative paths resolve beside the config, and mounts preserve absolute host paths.
  Read-only and writable mounts cannot overlap, including through symlinks. System
  directories and their aliases, the home volume, and `/tmp` are protected; subdirectories
  such as `/tmp/workspace` are allowed. Custom images must preserve these filesystem assumptions.
- **Provider:** `provider.base_url` must be a container-reachable OpenAI-compatible API.
  `provider.token` overrides `API_KEY`; an explicit empty token clears it.
- **MCP:** choose `mcp.url` for remote HTTP(S), or `mcp.command` for a stdio server running
  inside the container in the workspace, without shell parsing. Install dependencies in
  the image beforehand. Remote OAuth is disabled. Neither mode needs a published port;
  `mcp.port` only publishes an incoming server port on host loopback and does not start a server.
- **Access:** `container.web = true` enables the browser UI; the default is headless `serve`.
  Both use a generated host port and password. The default bind address is `0.0.0.0`;
  use `127.0.0.1` for host-only access. Remote access needs the host's reachable address
  and a trusted network or HTTPS ingress. Treat connection JSON as a credential.

On enforcing SELinux hosts, use pre-labeled mounts or explicitly select
`workspace.mount_label`. Relabeling changes host files, including read-only sources;
avoid system directories and whole home directories. Private labels are unsuitable for
mounts shared with other containers.

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

Run from the repository root:

```sh
go test -race -shuffle=on -count=1 ./...
go vet ./...
go build -o build/bob-manager ./cmd/container_manager
python3 -m unittest discover -s container -p network_test.py
```

Unit tests need no engine; firewall tests use host Python 3.11+. Docker integration tests
use synthetic HTTP/MCP servers and clean up their own containers and volumes:

```sh
docker build -t localhost/bob ./container
BOB_CONTAINER_TEST=1 go test -race ./...
```

Rebuild the image after bootstrap changes. See [CI workflows](../.github/workflows) for
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
