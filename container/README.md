# OpenCode container

This directory is the Docker/Podman image build context. Build from the repository root:

```sh
docker build -t localhost/bob ./container
```

Use the [container manager](../container_manager/README.md) to launch it with
validated mounts, identity mapping, endpoint allowlisting, and dynamic published
ports. No Compose setup is needed.

The entrypoint starts as container root, runs `network.py` to resolve configured
endpoints and install IPv4/IPv6 outbound firewall rules, then uses `setpriv` to
switch to the selected UID/GID, remove all capabilities, and enable
no-new-privileges. OpenCode becomes PID 1. Any bootstrap failure stops startup.

The launcher supplies these environment variables:

| Variable | Purpose |
| --- | --- |
| `BOB_EGRESS_URLS` | JSON list of HTTP(S) model-provider and remote MCP URLs |
| `BOB_UID`, `BOB_GID` | Agent filesystem identity; default 1000:1000 |
| `BOB_GROUPS` | Supplementary group IDs, or `keep-groups` for rootless Podman |
| `LLM_BASE_URL` | Required provider API URL |
| `LLM_MODEL_ID` | Provider model ID; default `Qwen3.8-27B` |
| `API_KEY` | Provider token, directly available to the agent |
| `OPENCODE_SERVER_PASSWORD` | Separate OpenCode HTTP Basic authentication password |
| `OPENCODE_CONFIG_CONTENT` | Optional local or remote MCP configuration |

Direct engine launches must start as root with NET_ADMIN, SETUID, SETGID, and
SETPCAP capabilities and provide `BOB_EGRESS_URLS`. Dropping all other capabilities,
providing writable `/tmp`, and setting no-new-privileges are part of the manager's launch configuration. Running the image
without firewall privileges or endpoint configuration fails rather than starting
an unrestricted agent.

The server listens on `0.0.0.0:4096`. Clients use HTTP Basic authentication with
username `opencode`. The writable `/home/opencode` volume stores configuration and
sessions. Additional mounts preserve host paths. Model and remote MCP servers
must use container-reachable addresses; `localhost` refers to the container.
With Docker, the manager maps `host.docker.internal` to the host gateway. Podman
provides `host.containers.internal` when supported by its network configuration.

Explicit arguments are forwarded to OpenCode after firewall setup. `EXPOSE` is
metadata: it does not publish ports or start MCP servers. Build arguments
`SERVER_PORT` and `MCP_PORT` customize that metadata; the Go manager uses port 4096.
