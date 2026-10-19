# OpenCode container

Build from the repository root, then launch with the
[container manager](../launcher/container_manager/README.md):

```sh
docker build -t localhost/bob ./containers
```

Rebuild after changes to the Dockerfile, entrypoint, firewall, or OpenCode configuration.
The image includes Node.js and Python for agent tasks. MCP servers run outside the
container; install their dependencies on their own host.

## Bootstrap

The entrypoint starts as root, verifies active seccomp filtering, and installs IPv4/IPv6
firewall rules through `network.py`. It then switches to the selected UID/GID, removes
all capabilities, and enables no-new-privileges. OpenCode becomes PID 1. Any bootstrap
failure prevents agent execution. See the manager's [network policy](../launcher/container_manager/README.md#network-policy).

The manager supplies provider settings (`LLM_BASE_URL`, `LLM_MODEL_ID`, `API_KEY`), the
endpoint allowlist (`BOB_EGRESS_URLS`), filesystem identity (`BOB_UID`, `BOB_GID`,
`BOB_GROUPS`), the generated `OPENCODE_SERVER_PASSWORD`, and optional MCP configuration and
model limits (`OPENCODE_CONFIG_CONTENT`). OpenCode merges these named MCP entries into the empty
`mcp` section in `/etc/opencode/opencode.json` at runtime, and a `provider.context_limit` from
bob.toml into the `self-hosted` model's `limit`. Prefer the manager for validated mounts and runtime settings.

The self-hosted model has a `no-thinking` variant that sends
`chat_template_kwargs: {"enable_thinking": false}`, which vLLM-served Qwen models honour.
A client selects it per message by sending `"variant": "no-thinking"` with the prompt;
without it the model reasons as usual.

The server listens on `0.0.0.0:4096` with HTTP Basic authentication as `opencode`.
`/home/opencode` holds persistent configuration and sessions. Provider/MCP addresses must
be reachable from inside the container: `localhost` refers to the container. Docker
launches map `host.docker.internal` to the host gateway; Podman provides
`host.containers.internal` where its network configuration supports it.

Explicit entrypoint arguments are forwarded to OpenCode after bootstrap. Build arguments
`SERVER_PORT` and `MCP_PORT` set exposed-port metadata; they do not publish ports or start
MCP servers. `SERVER_PORT` also changes the entrypoint's default port, but the manager
uses 4096.
