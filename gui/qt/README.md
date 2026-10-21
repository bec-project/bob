# bob desktop app (Qt)

A native Qt 6 / QML app for the OpenCode server that bob runs in its container, and the way bob
starts: the first app a user opens starts their bob, and later ones join it.

- **Chats** from the container, grouped by day, with search, rename and delete.
- **Streaming answers** with the model's thinking in a collapsible block and a live timer.
- **Tool calls** as rows with status, duration, input and output. A sub-agent's work (the `task`
  tool) shows inside its row as it happens.
- **Time and tokens** per answer and per chat, with a context meter and a *Compact* button.
- **Model, agent and thinking**: pickers for what OpenCode offers; *Thinking* off sends the
  model's `no-thinking` variant.
- **Approvals**: OpenCode's permission requests and questions appear above the composer.
- **The agent's plan**: its todo list sits above the composer while work is left.
- **Workspace changes**: the files each message changed, with their diffs, in the changes panel;
  *Undo* reverts a message and everything after it, *Redo* restores it.
- **Chat tools**: fork a chat (whole, or from any answer), copy it as Markdown or save it to
  Downloads.
- **Earlier prompts**: *Up* on the composer's first line steps back through the chat's prompts,
  *Down* forward again.
- **MCP servers**: one chip per server in the header, coloured by its state; a server that is not
  connected connects again when clicked.
- **Plugins**: views for the tools of the MCP servers bob.toml names, such as BEC's report cards,
  panels and scan approvals (see [Plugins](#plugins)).

The window talks to OpenCode directly over HTTP (REST with Basic auth and the `/event` stream).
The C++ side holds the lease on bob: it runs `bob-manager --config bob.toml`, connects to the
broker's socket, says hello and keeps the connection open while the app holds bob (see
[how bob runs](../../launcher/container_manager/README.md#how-bob-runs)). Quitting or crashing
ends the lease; bob stops 90 seconds after the last app that uses it is gone. When the broker ends
the lease (bob's container died, its broker was stopped, or another app restarted it), a bar above
the chat says why and offers to join again. When bob already runs with another bob.toml (another
program started it, or it still lingers after this app left it), the app joins it and asks whether
to restart bob with its own bob.toml. Restarting stops bob for every program that holds it, so the
dialog says how many others use it; *Restart with …* in Settings offers the same later.

The app keeps how it starts bob (the bob.toml, the executable, and whether to join when it opens)
in `settings.json`, in `~/.config/ch.psi.bob` on Linux and `~/Library/Application
Support/ch.psi.bob` on macOS: the same file as the Tauri desktop app, so both start bob
the same way. Look and layout are kept with QSettings. The executable defaults to `$BOB_BIN`, then
`bob-manager` beside the app, then (in a build from a checkout) the checkout's
`build/bob-manager` or `bob`, then `bob-manager` or `bob` on the PATH. bob-manager and bob's image are
installed separately.

## Build and run

Requires Qt 6.12 (Qt Quick, Qt Quick Controls, Qt Network) and CMake 3.22+. From the repository
root, with Qt's prefix in `CMAKE_PREFIX_PATH`:

```sh
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
cmake -S gui/qt -B build/qt -G Ninja -DCMAKE_PREFIX_PATH=$HOME/Qt/6.12.0/macos
cmake --build build/qt
build/qt/bob-gui.app/Contents/MacOS/bob-gui   # macOS; build/qt/bob-gui on Linux
```

In **Qt Creator**, open `gui/qt/CMakeLists.txt`. In **Qt Design Studio**, open
`gui/qt/bob.qmlproject`: it shows the QML module in `imports/Bob` with the stand-ins in
`designer/Bob/Backend` in place of the C++ backend, so the views can be edited without a bob.

For **QML live preview** (edits to the QML appear in the running app without a rebuild), build
with QML debugging, which a release build must never have: in Qt Creator, *QML debugging and
profiling* in the build settings, or `-DCMAKE_CXX_FLAGS_INIT=-DQT_QML_DEBUG`. Qt's
`QmlPreviewMcpServer` (Qt 6.12's Agentic Development Tools in the Maintenance Tool) then starts
the app with `run_qmlPreview`, given the app's executable
(`build/qt/bob-gui.app/Contents/MacOS/bob-gui`). To use it from Claude Code:

```sh
claude mcp add qmlpreview ~/Qt/6.12.0/macos/bin/QmlPreviewMcpServer
```

## Plugins

A plugin adds views for the tools of one kind of MCP server. bob.toml enables it for a server:

```toml
[mcp.bec_mcp]
url = "http://host.docker.internal:8000/mcp"
plugin = "bec"
```

The app makes one instance of the plugin per server that names it, so two servers of the same
kind get two instances, each with its own panels. An instance sees only its server: the tool
calls and approvals of that server's tools, and MCP calls to that server alone. Settings →
Plugins lists every instance, as "<plugin> · <server>", with a switch (on until you switch it
off) and the plugin's own settings; a plugin name the app does not have is listed there too.
Instances come from the chat's connection, so they stay while bob is lost; their calls reach only
the MCP servers of the bob this app holds, and fail with "join bob first" otherwise. A bob
connected by hand names no MCP servers, so it gets no plugins.

The BEC plugin (`plugin = "bec"`) adds report cards for a BEC MCP server's results, panels for
devices, log incidents and the queue, and readable scan approvals; see
[its README](imports/Bob/Plugins/Bec/README.md).

### Writing a plugin

A plugin is a QML module `Bob.Plugins.<Name>` in `imports/Bob/Plugins/<Name>/`, built as a static
library the app links, with one object (a `QtObject`) that describes it: `pluginId`, `name`,
`description`, prompt `suggestions`, side `panels`, a `settings` component for its row in
Settings, and `describeTool` and `describePermission`, which draw its server's tool calls (a
label, a summary and a body under the row) and permission requests. A line in
[`PluginRegistry.qml`](imports/Bob/PluginRegistry.qml)'s `known` registers it, and bob.toml names
it by that id; the file's header lists the whole interface. Plugins only add views; the chat
works without them.

Its logic is C++ in its own folder, registered with `QML_ELEMENT` (the BEC plugin's `BecCards`
and `BecServer`), and its QML only draws: the QML imports only Qt, the `Bob` module's controls and
theme, `Desktop` from `Bob.Backend` (to open links), and the plugin's own types. A plugin keeps its
docs, test data and Design Studio stand-ins for its C++ types in its folder, so adding or removing
one touches only that folder, its line in `known`, and the build (`add_subdirectory` and the link
in `imports/Bob/CMakeLists.txt`, `Q_IMPORT_QML_PLUGIN` in `src/main.cpp`, and its tests). Its
context carries its server, an `McpServer` (`src/gui/plugins.h`): `callTool` and `listTools` from C++,
or `call(tool, args, callback)` from QML, go through the app's C++ MCP client, which looks the
server up in the connection of the bob this app holds, keeps one MCP session per server and calls
only `http://` servers on loopback, never through a proxy. A call gives up after 2 minutes unless
the caller allows more.

## Layout

| Path | What it holds |
| --- | --- |
| `src/bob/` | Talking to bob, without QML (library `bobcore`): the lease on bob (`bobsession`), its settings, running containers, the OpenCode client and event stream, the chat's sessions they keep current (`chatclient`), and the MCP client the plugins call through (`mcpclient`) |
| `src/gui/` | What the views read, QML module `Bob.Backend` on top of `bobcore`: the chat as the window shows it (`chat`), the models, preferences, desktop helpers and the plugins' MCP servers (`plugins`) |
| `imports/Bob/` | The window, QML module `Bob`; `qmldir` lets Design Studio read it from the source tree |
| `imports/Bob/Plugins/` | The plugins, one QML module each |
| `designer/` | Design Studio's preview and stand-ins for `Bob.Backend` |
| `tests/` | Unit tests (Qt Test), the BEC plugin's cards included |
| `packaging/` | The RPM spec and the desktop entry |

## Checks

```sh
cmake --build build/qt && (cd build/qt && QT_QPA_PLATFORM=offscreen ctest --output-on-failure)
```

## Released bundles

Each bob release also carries this app, built by [`qt_desktop.yml`](../../.github/workflows/qt_desktop.yml),
on the GitHub release and in Gitea (package `bob`, and the RPM in the `bec` RPM registry):

- `bob-gui-<version>-1.el9.x86_64.rpm`: RHEL 9. RHEL 9 has no Qt 6.12 (EPEL 9 ships 6.10), so the
  RPM carries Qt's official binaries in `/opt/bob-gui`, with `bob-gui` on the PATH. It needs only
  base RHEL 9 packages.
- `bob-gui_<version>_darwin_arm64.dmg`: macOS on Apple silicon, with Qt inside the app. It is
  signed ad hoc, so macOS blocks its first start: allow it under System Settings → Privacy &
  Security, or run `xattr -dr com.apple.quarantine /Applications/bob-gui.app`.
