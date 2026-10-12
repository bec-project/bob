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

## Documentation

- [Configuration example](bob.example.toml) — settings and defaults
- [Container manager](container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Container image](container/README.md) — building and bootstrap

The firewall filters IP addresses and ports, not domains or URL paths. Review the
[isolation limits](container_manager/README.md#isolation-limits) before choosing mounts.
