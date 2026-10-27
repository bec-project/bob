# BOB

Run OpenCode on your Linux machine in a bubblewrap sandbox, with persistent sessions and no
network but your configured model provider and remote MCP servers.

## Get started

Building bob-manager requires Go 1.25+. Running bob requires Linux with `bwrap` on the PATH
(RHEL 9 package `bubblewrap`), unprivileged user namespaces, and an OpenCode executable. From the
repository root:

```sh
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
cp deploy/bob.example.toml bob.toml
```

Edit `bob.toml`: set `workspace.workdir` to your project directory outside this repository
and `provider.base_url` to your OpenAI-compatible API. Set the model and token as needed, and
`sandbox.opencode` if OpenCode is not on the PATH.

Each user has one bob, started by a client of the user's broker: the first client starts it,
every other client of that user joins the same bob and sees the same sessions, and bob stops 90
seconds after the last of them is gone; sessions remain in bob's home folder (`sandbox.home`). See
[how bob runs](launcher/container_manager/README.md#how-bob-runs) for the client protocol. Treat
the connection a client receives as a credential. For host-only access, set
`sandbox.bind_address = "127.0.0.1"` (the default accepts connections on all interfaces).

## Repository layout

| Path | What it holds |
| --- | --- |
| `launcher/` | Go module: the `bob-manager` CLI (`cmd/container_manager`) and bob's sandbox manager, package `containermanager` |
| `gui/qt/` | The desktop app (Qt 6, C++ and QML): starts and holds bob, and chats with it, with plugins for MCP servers such as BEC; RPM for RHEL 9 and a macOS DMG |
| `deploy/` | Deployment configuration: the annotated `bob.example.toml` |
| `ci/`, `.github/` | Release tooling and CI workflows |

## Documentation

- [Configuration example](deploy/bob.example.toml) — settings and defaults
- [Sandbox manager](launcher/container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Desktop app](gui/qt/README.md) — features, plugins, building, Qt Creator and Design Studio, released bundles

The sandbox reaches only the configured endpoints, filtered by host and port, not URL path, and
the agent runs as your own user. Review the
[isolation limits](launcher/container_manager/README.md#isolation-limits) before choosing mounts.
