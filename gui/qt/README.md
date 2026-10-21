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

The window talks to OpenCode directly over HTTP (REST with Basic auth and the `/event` stream).
The C++ side holds the lease on bob: it runs `bob-manager --config bob.toml`, connects to the
broker's socket, says hello and keeps the connection open while the app holds bob (see
[how bob runs](../../launcher/container_manager/README.md#how-bob-runs)). Quitting or crashing
ends the lease; bob stops 90 seconds after the last app that uses it is gone. When the broker ends
the lease (bob stopped), a bar above the chat says so and offers to join again.

The app keeps how it starts bob (the bob.toml, the executable, and whether to join when it opens)
in `settings.json`, in `~/.config/ch.psi.bob` on Linux and `~/Library/Application
Support/ch.psi.bob` on macOS: the same file as the Tauri desktop app, so both start bob
the same way. Look and layout are kept with QSettings. The executable defaults to `$BOB_BIN`, then
`bob-manager` beside the app, then (in a build from a checkout) the checkout's
`build/bob-manager`, then `bob-manager` or `bob` on the PATH. bob-manager and bob's image are
installed separately.

## Build and run

Requires Qt 6.12 (Qt Quick, Qt Quick Controls, Qt Network) and CMake 3.22+. From the repository
root, with Qt's prefix in `CMAKE_PREFIX_PATH`:

```sh
go -C launcher build -o ../build/bob-manager ./cmd/container_manager
cmake -S gui/qt -B build/qt -G Ninja -DCMAKE_PREFIX_PATH=$HOME/Qt/6.12.0/macos
cmake --build build/qt
build/qt/src/bob-gui.app/Contents/MacOS/bob-gui   # macOS; build/qt/src/bob-gui on Linux
```

In **Qt Creator**, open `gui/qt/CMakeLists.txt`. In **Qt Design Studio**, open
`gui/qt/bob.qmlproject`: it shows the QML module in `imports/Bob` with the stand-ins in
`designer/Bob/Backend` in place of the C++ backend, so the views can be edited without a bob.

## Layout

| Path | What it holds |
| --- | --- |
| `src/` | The C++ backend, QML module `Bob.Backend`: the lease on bob (`bobsession`), its settings, running containers, the OpenCode client and event stream, the chat state (`chat`) and the models the views read |
| `imports/Bob/` | The window, QML module `Bob`; `qmldir` lets Design Studio read it from the source tree |
| `designer/` | Design Studio's preview and stand-ins for `Bob.Backend` |
| `tests/` | Unit tests (Qt Test) |
| `packaging/` | The desktop entry and AppStream metadata |

## Checks

```sh
cmake --build build/qt && (cd build/qt && QT_QPA_PLATFORM=offscreen ctest --output-on-failure)
```

