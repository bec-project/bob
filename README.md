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
The example uses Docker and enables the browser UI.

```sh
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
build/bob-manager --config bob.toml
```

Open the returned URL. Treat the connection output as a credential. Ctrl-C removes the
container; sessions remain in the home volume. For host-only access, set
`container.bind_address = "127.0.0.1"` (the default accepts connections on all interfaces).

With `--shared`, every program of a user shares one bob: the first one starts it, the others
attach, and it stops after the last one is gone. See [shared mode](launcher/container_manager/README.md#shared-mode).

## Repository layout

| Path | What it holds |
| --- | --- |
| `launcher/` | Go module: the `bob-manager` CLI (`cmd/container_manager`) and the `containermanager` package |
| `containers/` | What bob ships as containers: the OpenCode image's Dockerfile, firewall bootstrap, entrypoint and OpenCode configuration |
| `deploy/` | Deployment configuration: the annotated `bob.example.toml` |
| `ci/`, `.github/` | Release tooling and CI workflows |

## Documentation

- [Configuration example](deploy/bob.example.toml) — settings and defaults
- [Container manager](launcher/container_manager/README.md) — configuration, network policy, isolation limits, and testing
- [Container image](containers/README.md) — building and bootstrap

The firewall filters IP addresses and ports, not domains or URL paths. Review the
[isolation limits](launcher/container_manager/README.md#isolation-limits) before choosing mounts.
