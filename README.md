# BOB

Run OpenCode on your Linux machine in a sandbox-runtime (bubblewrap) sandbox, with persistent
sessions and no network but your configured model provider and remote MCP servers.

## Get started

Building bob-manager requires Go 1.25+. Running bob requires Linux with `bwrap`, `socat` and
Node.js 22.12+ on the PATH (RHEL 9 packages `bubblewrap`, `socat` and the `nodejs:22` module
stream), unprivileged user namespaces, Anthropic's sandbox-runtime, ripgrep, and an OpenCode
executable. From the repository root:

```sh
go -C launcher build -o ../build/bob-manager ./cmd/bob-manager
cp deploy/bob.example.toml bob.toml
```

Edit `bob.toml`: set `provider.base_url` to your OpenAI-compatible API, then the model and
token as needed. The MCP-only profile rejects workspace mounts: source access belongs behind
your MCP server. Set `sandbox.opencode` when the pinned OpenCode is not installed with bob-manager.

Each user has one bob, started by a client of the user's broker: the first client starts it,
every other client of that user joins the same bob and sees the same sessions, and bob stops 90
seconds after the last of them is gone; sessions remain in bob's home folder (`sandbox.home`). See
[how bob runs](launcher/sandbox/README.md#how-bob-runs) for the client protocol. Treat
the connection a client receives as a credential. The API binds to `127.0.0.1` by default.
Remote access requires an explicit bind address and protected TLS ingress.

## Repository layout

| Path | What it holds |
| --- | --- |
| `launcher/` | Go module: the `bob-manager` CLI (`cmd/bob-manager`) and bob's sandbox manager, package `sandbox` |
| `gui/qt/` | The desktop app (Qt 6, C++ and QML): starts and holds bob, and chats with it; RPM for RHEL 9 and a macOS DMG |
| `deploy/` | Deployment configuration: the annotated `bob.example.toml` |
| `ci/`, `.github/` | Release tooling and CI workflows |

## Documentation

- [Configuration example](deploy/bob.example.toml) — settings and defaults
- [Sandbox manager](launcher/sandbox/README.md) — configuration, network policy, isolation limits, and testing
- [Desktop app](gui/qt/README.md) — features, building, Qt Creator and Design Studio, released bundles

The sandbox reaches only the configured endpoints, filtered by host and port, not URL path, and
the agent runs as your own user. Review the
[isolation limits](launcher/sandbox/README.md#isolation-limits) before deployment.
