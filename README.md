# BOB

Run OpenCode in an isolated Docker or Podman container, with persistent sessions and
an outbound firewall limited to your configured model provider and remote MCP servers.

## Get started

Requires Go 1.25+ and a running Docker or Podman engine. From the repository root:

```sh
docker build -t localhost/bob ./containers
cp deploy/bob.example.toml bob.toml
```

Edit `bob.toml`: set `workspace.workdir` to your project directory outside this repository
and `provider.base_url` to your OpenAI-compatible API. Set the model and token as needed.
The example uses Docker.

Each user has one bob, started by a client of the user's broker: the first client starts it,
every other client of that user joins the same bob and sees the same sessions, and bob stops 90
seconds after the last of them is gone; sessions remain in the home volume. See
[how bob runs](launcher/container_manager/README.md#how-bob-runs) for the client protocol. Treat
the connection a client receives as a credential. For host-only access, set
`container.bind_address = "127.0.0.1"` (the default accepts connections on all interfaces).

## Repository layout

| Path | What it holds |
| --- | --- |
| `launcher/` | Go module: the `bob-manager` CLI (`cmd/container_manager`) and the `containermanager` package |
| `containers/` | What bob ships as containers: the OpenCode image's Dockerfile, firewall bootstrap, entrypoint and OpenCode configuration |
| `gui/qt/` | The desktop app (Qt 6, C++ and QML): starts and holds bob, and chats with it, with plugins for MCP servers; RPM for RHEL 9 and a macOS DMG |
| `deploy/` | Deployment configuration: the annotated `bob.example.toml` |
| `ci/`, `.github/` | Release tooling and CI workflows |

## Documentation

- [Configuration example](deploy/bob.example.toml) — settings and defaults
- [Container manager](launcher/container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Container image](containers/README.md) — building and bootstrap
- [Desktop app](gui/qt/README.md) — features, plugins, building, Qt Creator and Design Studio, released bundles

The firewall filters IP addresses and ports, not domains or URL paths. Review the
[isolation limits](launcher/container_manager/README.md#isolation-limits) before choosing mounts.
