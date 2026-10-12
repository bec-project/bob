# BOB

BOB launches one OpenCode container through Docker or Podman. An outbound firewall
restricts new external TCP connections to the configured model-provider and remote MCP
server IP addresses and ports. See the [network
policy](container_manager/README.md#network-policy) for local traffic and other
permitted network operations.

| Directory | Purpose |
| --- | --- |
| [cmd/container_manager](cmd/container_manager/main.go) | Executable: arguments, TOML loading, and shutdown |
| [container_manager](container_manager/README.md) | Reusable lifecycle, configuration resolution, and runtime arguments |
| [container](container/README.md) | OpenCode image and firewall bootstrap |

Requires Go 1.25+ and a container engine. From the repo root:

```sh
docker build -t localhost/bob ./container
cp bob.example.toml bob.toml
# Edit bob.toml: choose the engine, working directory, and provider URL.
go run ./cmd/container_manager
```

Set `web = true` in the TOML `[container]` section to enable OpenCode's browser UI on
the published port. The default is headless `serve`.

Use `--config PATH` to choose another file. See [bob.example.toml](bob.example.toml) for
annotated settings and [container_manager](container_manager/README.md) for
configuration and network scope. The launcher prints the authenticated OpenCode
connection and removes its container on shutdown; home storage persists.

Build and verify:

```sh
go build -o build/bob-manager ./cmd/container_manager
go test -race ./...
go vet ./...
python3 -m unittest discover -s container -p network_test.py
BOB_CONTAINER_TEST=1 go test -race ./...
```

The firewall unit tests use host Python 3. Integration tests require Docker and the
built agent image. They use synthetic HTTP/MCP servers and clean up their containers and
volumes. Podman needs testing on the deployment host, particularly rootless networking
and supplementary groups.

GitHub Actions runs formatting/static checks, Go race tests on Linux/macOS, Python-version
matrix tests, Docker integration tests, and cross-platform binary builds. Successful main
CI runs trigger semantic releases using the standard semantic-release CLI and `.releaserc.json`.
Binaries are attached to GitHub releases; the multi-platform agent image is published as
`gitea.psi.ch/bec/bob:<version>` and `gitea.psi.ch/bec/bob:latest`. Node is needed only for
release tooling; dependencies are locked in `package-lock.json`. GitHub releases use the
built-in `GITHUB_TOKEN`. Container publishing requires the repository or organization secret
`GITEA_REGISTRY_TOKEN` in `username:token` format, with permission to publish packages under
the Gitea `bec` organization, matching the BEC and BEC log ingestor workflows.
Go and Python unit-test jobs generate coverage reports and upload them to Codecov using
the repository or organization secret `CODECOV_TOKEN`.
