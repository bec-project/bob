# BOB

Run OpenCode in an isolated Docker or Podman container, with persistent sessions and
an outbound firewall limited to your configured model provider and remote MCP servers.

## Get started

Requires Go 1.25+ and a running Docker or Podman engine. From the repository root:

```sh
docker build -t localhost/bob ./container
cp bob.example.toml bob.toml
```

Edit `bob.toml`: set `workspace.workdir` to your project directory outside this repository
and `provider.base_url` to your OpenAI-compatible API. Set the model and token as needed.
The example uses Docker and enables the browser UI.

```sh
go run ./cmd/container_manager --config bob.toml
```

Open the returned URL. Treat the connection output as a credential. Ctrl-C removes the
container; sessions remain in the home volume. For host-only access, set
`container.bind_address = "127.0.0.1"` (the default accepts connections on all interfaces).

With `--shared`, every program of a user shares one bob: the first one starts it, the others
attach, and it stops after the last one is gone. See [shared mode](container_manager/README.md#shared-mode).

For bob's own chat GUI instead of OpenCode's browser UI, start both together (Node 20+):

```sh
npm --prefix gui install
npm --prefix gui run bob -- --config bob.toml
```

It shows the container's sessions, the model's thinking, tool calls, time and token usage,
and a thinking switch; a BEC plugin adds report cards and a device panel. It also runs as a
Tauri desktop app. See the [GUI guide](gui/README.md).

## Documentation

- [Configuration example](bob.example.toml) — settings and defaults
- [Container manager](container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Container image](container/README.md) — building and bootstrap
- [GUI](gui/README.md) — the chat UI, its desktop app and plugins

The firewall filters IP addresses and ports, not domains or URL paths. Review the
[isolation limits](container_manager/README.md#isolation-limits) before choosing mounts.
