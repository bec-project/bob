# BOB

Run OpenCode in an isolated Docker or Podman container, with persistent sessions and
an outbound firewall limited to your configured model provider and remote MCP servers.

## Get started

Requires Go 1.25+, Node 20+, a Rust toolchain with the
[Tauri prerequisites](https://tauri.app/start/prerequisites/), and a running Docker or Podman
engine. From the repository root:

```sh
docker build -t localhost/bob ./containers
cp deploy/bob.example.toml bob.toml
```

Edit `bob.toml`: set `workspace.workdir` to your project directory outside this repository
and `provider.base_url` to your OpenAI-compatible API. Set the model and token as needed.
The example uses Docker.

bob starts from its GUI, a Tauri desktop app:

```sh
npm --prefix gui/web install
npm --prefix gui/web run bob
```

The app starts bob with the `bob.toml` at the repository root; pick another one on its connect
screen, and the app remembers it.

It shows the container's sessions, the model's thinking, tool calls, time and token usage,
and a thinking switch. See the [GUI guide](gui/README.md).

Each user has one bob. The first GUI starts it, every other GUI or program of that user joins
the same bob and sees the same sessions, and bob stops 90 seconds after the last of them is
gone; sessions remain in the home volume. See [how bob runs](launcher/container_manager/README.md#how-bob-runs).
For host-only access, set `container.bind_address = "127.0.0.1"` (the default accepts
connections on all interfaces).

## Repository layout

| Path | What it holds |
| --- | --- |
| `launcher/` | Go module: the `bob-manager` CLI (`cmd/container_manager`) and the `containermanager` package |
| `containers/` | What bob ships as containers: the OpenCode image's Dockerfile, firewall bootstrap, entrypoint and OpenCode configuration |
| `gui/web/` | The chat GUI's page (React, Vite) |
| `gui/desktop/` | The Tauri desktop app around it, which starts and holds bob |
| `deploy/` | Deployment configuration: the annotated `bob.example.toml` |
| `ci/`, `.github/` | Release tooling and CI workflows |

## Documentation

- [Configuration example](deploy/bob.example.toml) — settings and defaults
- [Container manager](launcher/container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Container image](containers/README.md) — building and bootstrap
- [GUI](gui/README.md) — the chat UI and its desktop app

The firewall filters IP addresses and ports, not domains or URL paths. Review the
[isolation limits](launcher/container_manager/README.md#isolation-limits) before choosing mounts.
