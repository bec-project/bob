# Container manager

Import `bob/container_manager` (package `containermanager`) to run one container per
`Manager`. Calls to a manager must be serialized. Executable and TOML handling live in
`cmd/container_manager`.

Read the implementation in this order:

| File | Responsibility |
| --- | --- |
| `options.go` | Options, defaults, validation, resolution, and mount checks |
| `launch.go` | Complete creation arguments, environment, and caller identity preparation |
| `manager.go` | Lifecycle, connection details, endpoint discovery, and readiness |
| `runtime.go` | Executable lookup and subprocess execution/cancellation |
| `../container/network.py` | Outbound firewall bootstrap |

The flow is `getOptions` → `loadConfig` → `Manager.Start`. Start resolves settings,
checks the engine, prepares home-volume ownership, creates and starts the container,
discovers published endpoints, and waits for readiness. Stop force-removes the container
while preserving the home volume. Ctrl-C cancels startup or shuts down a running
container; further interrupts do not interrupt cleanup. Cleanup errors produce a nonzero
exit status. An existing container with a conflicting name is never removed.
Each launch carries a random ownership label, allowing cleanup to find its container
even when creation was accepted but a cancelled engine command did not return its ID.
Cleanup waits up to 30 seconds for a container from a failed create to appear, including
transport and process failures. Failures before dispatch and recognized Docker validation
rejections are settled; cleanup still checks for any owned container left behind. Unknown
daemon responses, including proxy and server timeouts, remain uncertain.
Configured names are checked for conflicts before creation. The image must already be
available in the selected engine: build or pull it before launching. Creation never pulls.
If creation remains unresolved, it reports an error and retains ownership for a later
`Stop` retry; another `Start` is blocked until cleanup succeeds. An empty lookup alone
does not settle an interrupted create. If removal reports an error, cleanup independently
checks whether the owned container is gone before clearing its state.
Linux Docker home-ownership preparation uses a separately labeled helper container with
the same cleanup tracking. It checks the helper's exit status before starting the agent.

## Configuration

The launcher reads `bob.toml`, or the file selected with `--config PATH`.
[bob.example.toml](../bob.example.toml) documents every setting. With no arguments and
no default file, it prints help. Replace the example provider URL before launching.
Missing required fields and invalid settings fail before container startup; endpoint
resolution and firewall setup run inside the container during bootstrap and prevent
OpenCode from starting if they fail.

| TOML field | Purpose / default |
| --- | --- |
| `container.runtime` | `docker` or `podman`; default `podman` |
| `container.web` | Use `opencode web` for the browser UI; default `false` (`serve`) |
| `container.image` | Agent image; default `localhost/bob` |
| `container.home_volume` | Persistent storage; default `bob-cli-home` |
| `container.name` | Container name; generated when omitted |
| `container.bind_address` | OpenCode API bind IP; default `0.0.0.0` |
| `workspace.workdir` | Required working directory, mounted writable |
| `workspace.rw` | Additional writable directories; default `[]` |
| `workspace.ro` | Additional read-only directories; default `[]` |
| `workspace.mount_label` | Optional SELinux relabeling: `shared` (`:z`) or `private` (`:Z`); omitted for pre-labeled mounts |
| `provider.base_url` | Required OpenAI-compatible HTTP(S) API URL; no default |
| `provider.model` | Model ID; default `Qwen3.8-27B` |
| `provider.token` | Overrides `API_KEY`; an explicit empty string clears it |
| `mcp.command` | Optional executable/argument array for a local stdio server |
| `mcp.url` | Optional remote HTTP(S) MCP endpoint; mutually exclusive with `command` |
| `mcp.port` | Optional incoming MCP container port, e.g. `19000`; default `0` (disabled) |

Relative directory paths resolve beside the TOML file. Mounts retain their absolute host
paths inside the agent. RO mounts cannot overlap writable mounts, including through
symlinks. Mounts cannot hide the image's system directories or home volume.
The `/tmp` destination is reserved for the container's tmpfs; subdirectories such as
`/tmp/workspace` remain usable. The bundled image's system-directory aliases, including
`/var/run`, `/var/lock`, and their parents, are also protected. Custom images must preserve
these filesystem assumptions.
On enforcing SELinux hosts, use pre-labeled directories or explicitly set `mount_label`.
Relabeling changes host files, including read-only mount sources; do not select it for
system directories or a whole home directory used by other confined services. Private
labels are unsuitable for directories shared with other containers.

The bundled OpenCode configuration enables one self-hosted provider through
`@ai-sdk/openai-compatible`. The provider must implement that API protocol.

OpenCode manages the configured MCP server, named `bob`. Commands run inside the agent
in `workspace.workdir`, with arguments passed without shell parsing. Install
dependencies in the image beforehand; arbitrary internet downloads are blocked. Remote
and stdio MCP servers do not need published ports. Remote MCP OAuth is disabled.

Go callers start with `DefaultOptions()`, then supply the working directory, provider
URL, and key. Docker uses the caller's UID/GID and supplementary groups when the
launcher runs on Linux; on other host platforms it uses UID/GID 1000. Podman uses
`keep-id` and Linux `keep-groups` (requires `crun`). Bootstrap runs as container root;
OpenCode runs with the selected filesystem identity and no capabilities.
Docker rootless and daemon `userns-remap` modes are rejected because host IDs do not map
directly to container IDs; use rootless Podman with `keep-id` for that setup. Docker home
preparation updates existing state recursively without following symlinks. Do not share
one home volume concurrently between launches using different UID/GID identities.

The launcher explicitly selects Docker's built-in seccomp profile
(`--security-opt seccomp=builtin`) or Podman's default policy
(`--seccomp-policy default`). The same policy applies to home-ownership preparation.
The image verifies active filtering before network setup and refuses unconfined startup.
Use a current Docker client/engine supporting `seccomp=builtin`; older clients may
interpret `builtin` as a filename. Rebuild the image after updating its bootstrap code.

## Network policy

Before running agent code, bootstrap resolves every configured endpoint, pins its
addresses in `/etc/hosts`, and installs default-deny IPv4 and IPv6 OUTPUT policies.
Allowed traffic is TCP to those IP addresses on the endpoint's port (443 for HTTPS, 80
for HTTP when omitted), local TCP on 127.0.0.1/::1, and TCP replies to incoming
connections. IPv6 neighbour discovery is allowed for network operation. DNS and other
UDP traffic are blocked after startup, including Docker's embedded DNS service.
Unresolvable endpoints or failed firewall setup prevent OpenCode from starting. Restart
to refresh endpoint IP addresses after DNS changes.

The policy matches IP addresses and ports, not domain names or HTTP paths. Other paths
and domains served by the same permitted IP/port are reachable. The upstream server must
enforce any URL-path restrictions and credential scope. Redirects to a different
destination are blocked unless it shares an allowed address and port. Provider
credentials are passed directly through the agent's environment and are visible to the
agent and engine administrators.

Bootstrap receives only NET_ADMIN, SETUID, SETGID, and SETPCAP, then drops all
capability sets and enables no-new-privileges before OpenCode starts. Image system files
are root-owned; the home volume, `/tmp`, and configured RW mounts are writable by the
selected agent identity. No host firewall rules are installed. Users with engine
administration access can override the container policy.

Set `container.web = true` to launch `opencode web` instead of `serve` using the same
image and published port. Both commands listen on `0.0.0.0:4096` inside the container
and use the generated server password. Open the returned URL in a browser; remote
clients use the container host's reachable address and published port.
`container.bind_address` determines which host interfaces accept connections.

OpenCode's host port and password are generated automatically. With the default
`0.0.0.0` bind address, the returned URL uses localhost. A specific bind IP remains in
the returned URL. Remote clients use the host's reachable address and the same published
port. The incoming MCP port binds to host loopback. Configure one integer `mcp.port`, or
omit it or use `0` to disable publishing. Connection JSON exposes its address as
`mcpPort` (an empty string when disabled). Treat the connection record as a credential.
Use `container.bind_address = "127.0.0.1"` for host-only OpenCode access. Remote access
uses authenticated HTTP; use a trusted network or HTTPS ingress.

Docker is integration-tested. Podman and Linux filesystem identity mapping need
verification on their deployment hosts. Firewall setup requires NET_ADMIN inside the
container's own network namespace; unsupported runtimes fail closed.

## Isolation limits

The agent loses all bootstrap capabilities and runs with no-new-privileges. This prevents
it from changing the firewall or regaining privileges through setuid executables. It
does not make arbitrary host mounts safe or establish resistance to kernel/runtime bugs.
Use a maintained, patched container engine/runtime and enable its seccomp policy.
For example, runc versions through 1.1.11 are affected by the container escape
[CVE-2024-21626](https://github.com/opencontainers/runc/security/advisories/GHSA-xr7r-f8xq-vfvv);
the fix for that issue starts at 1.1.12. This is not a substitute for keeping the runtime
current against later vulnerabilities.

Keep the launcher, its configuration, and other host-executed code outside the agent's
writable workspace. The agent can modify files in writable mounts, including scripts or
Git hooks that the host may later execute. Do not expose Docker/Podman sockets or other
host-control sockets, even through read-only mounts: socket requests are not read-only
filesystem writes. Linux supplementary groups are inherited, so a mounted socket may
be accessible through the caller's groups. Avoid launching as host root.

Configured provider/MCP endpoints and replies to incoming TCP connections remain usable
communication channels. Unix sockets are outside the IP firewall. No CPU, memory, or PID
limits are configured, so resource exhaustion is also outside the current policy.
