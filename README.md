# BOB

BOB launches one OpenCode container through Docker or Podman. An outbound firewall
allows only the configured model-provider and remote MCP server IP addresses and
TCP ports. No Compose provider or proxy container is required.

| Directory | Purpose |
| --- | --- |
| [cmd/container_manager](cmd/container_manager/main.go) | Executable: arguments, TOML loading, and shutdown |
| [container_manager](container_manager/README.md) | Reusable lifecycle, configuration resolution, and runtime arguments |
| [container_manager_py](container_manager_py/README.md) | Python alternative using the same TOML and container image |
| [container](container/README.md) | OpenCode image and firewall bootstrap |
| [cmd/bob_log_service](cmd/bob_log_service/main.go) | Empty entry point for future log-service development |

Requires Go 1.25+ and a container engine. From the repo root:

```sh
docker build -t localhost/bob ./container
cp bob.example.toml bob.toml
# Edit bob.toml: choose the engine, working directory, and provider URL.
go run ./cmd/container_manager
```

For the Python alternative (Python 3.11+, no extra dependencies), run
`python3 -m container_manager_py --config bob.toml`.

Set `web = true` in the TOML `[container]` section to enable OpenCode's browser UI
on the published port. The default is headless `serve`.

Use `--config PATH` to choose another file. See [bob.example.toml](bob.example.toml)
for annotated settings and [container_manager](container_manager/README.md) for
configuration and network scope. The launcher prints the authenticated OpenCode
connection and removes its container on shutdown; home storage persists.

Build and verify:

```sh
go build -o build/bob-manager ./cmd/container_manager
go build -o build/bob_log_service ./cmd/bob_log_service
go test -race ./...
go vet ./...
python3 -m unittest discover -s container -p network_test.py
python3 -m unittest container_manager_py.tests
BOB_CONTAINER_TEST=1 go test -race ./...
```

Integration tests require Docker and the built agent image. They use synthetic
HTTP/MCP servers and clean up their containers and volumes. Podman needs testing
on the deployment host, particularly rootless networking and supplementary groups.
