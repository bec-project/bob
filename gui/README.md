# bob GUI

A chat UI for the OpenCode server that bob runs in its container, and the way bob starts: the
first GUI a user opens starts their bob, and later ones join it.

- **Sessions** from the container, grouped by day, with search, rename and delete.
- **Streaming answers** with the model's thinking in a collapsible block and a live timer.
- **Tool calls** as rows with status, duration, input and output. A sub-agent's work (the `task`
  tool) shows inside its row as it happens.
- **Time and tokens** per answer (input, output, thinking, steps, tool calls) and per chat, with
  a context meter (see [Context](#context)) and a *Compact* button that summarises the chat.
- **Model and thinking**: the model picker lists what OpenCode offers; the *Thinking* switch
  sends the model's `no-thinking` variant when it is off (see [Thinking](#thinking)).
- **Approvals**: OpenCode's permission requests (a shell command, an edit, an MCP tool marked
  `ask`) and questions appear above the composer; *Allow once*, *Always allow* or *Reject*.
- **The agent's plan**: its todo list sits above the composer while work is left.
- **Workspace changes**: the files each message changed, with their diffs, in the changes panel
  (the file icon on the right). *Undo* on a message reverts it and everything after it, files
  included, and puts its text back in the composer; *Redo* restores it. OpenCode tracks changes
  with git, so this needs the workspace to be a git repository.
- **Chat tools**: fork a chat (whole, or from any answer), copy it as Markdown or save it as a
  `.md` file (the desktop app saves to Downloads).
- **Plugins**, off by default. The BEC plugin is described [below](#bec-plugin).

The same frontend runs in a browser and as a [Tauri](https://tauri.app) desktop app.

## Run it

Requires Node 20+ (and Go, to build bob). From the repository root:

```sh
npm --prefix gui/web install
npm --prefix gui/web run bob -- --config bob.toml
```

This builds bob, joins your bob (starting it when none runs), waits until it is up and opens
the GUI on <http://127.0.0.1:5173>, already connected. Ctrl-C stops the GUI and ends its hold on
bob; bob stops 90 seconds after the last program that uses it is gone. The connection holds
OpenCode's password; the launcher passes it to the GUI through a private temporary file
(`BOB_CONNECTION_FILE`) that it deletes on exit. The dev server, which listens on 127.0.0.1
only, hands that connection to its page, and so to any local process that asks for it: on a
host shared with other users, use the desktop app instead.

To connect to a bob running elsewhere, run `npm --prefix gui/web run dev` and paste its
connection (URL and password) into the connect screen.

### Desktop app

The desktop app needs a Rust toolchain and the
[Tauri prerequisites](https://tauri.app/start/prerequisites/) for your platform (Xcode command
line tools on macOS; WebKitGTK on Linux).

```sh
npm --prefix gui/web run bob -- --config bob.toml --desktop   # bob + the app, in development mode
npm --prefix gui/web run tauri build                          # a bundle in gui/desktop/target/release/bundle
```

The desktop app uses your bob (see [how bob runs](../launcher/container_manager/README.md#how-bob-runs)):
when it opens, it joins the bob you already run, or starts it with the remembered bob.toml when
none runs, unless *Do this when the app opens* is off. It holds bob until you quit it or choose
*Leave this bob* in Settings; bob stops 90 seconds after the last program that uses it (this
app, the browser GUI, BEC widgets) is gone. A crash ends the hold too.

The connect screen sets the bob.toml and the bob executable. The executable defaults to
`$BOB_BIN`, then `build/bob-manager` of the source tree the app was built from (`npm run bob`
builds it), then `bob` on the PATH. The screen also lists every running bob container on Docker
or Podman; connecting to one of those does not hold it. To read such a container's OpenCode
password the app uses `docker inspect`, which only someone who can already control the engine can
do. Settings → *Switch bob…* goes back to that screen.

`npm run bob` joins your bob the same way, so the browser GUI and the desktop app use one bob.

## Thinking

OpenCode switches thinking off through a model *variant*. bob's image defines one for the
self-hosted model in [`image/opencode.json`](../image/opencode.json):

```json
"variants": { "no-thinking": { "chat_template_kwargs": { "enable_thinking": false } } }
```

`chat_template_kwargs` is what vLLM-served Qwen models honour (PSI's gateway included). The
switch is greyed out for a model without a variant whose name contains `no-thinking`; rebuild
the image (`docker build -t localhost/bob ./image`) to get it.

## Context

bob tells OpenCode the model's context window when `provider.context_limit` is set in bob.toml
(PSI's gateway reports 262144 tokens for Qwen3.8-27B). The usage bar then shows how full the
context of the last model call was, and OpenCode compacts a chat on its own before it overflows.
*Compact* does it on demand: the chat continues from a summary, shown as *Summary of the earlier
chat*.

## BEC plugin

Switch it on in Settings → Plugins. Tool ids are matched by suffix, so `[mcp.bec_mcp]` (tool
`bec_mcp_bec_devices`) and any other server name work.

- **Report cards** for BEC MCP results in the chat: services, devices, a reading against its
  limits, the queue, scan history, logs, diagnosis and docs. The log analyser's incidents show
  their likely cause, suggestion, origin line and the records around them; `bec_log_range`
  records show their tracebacks; in code mode, `bec_code_traceback` shows every frame with its
  code and the raising line marked. *Ask* on a finding or incident puts a follow-up question
  into the composer.
- **Panels** (icons on the right) that call the BEC MCP server themselves:
  - *Devices*: every device; a click reads it (`bec_device_read`).
  - *Log incidents*: `bec_log_analysis` for a time window, refreshed every 30 s, with *Show
    records* (`bec_log_range`) and, when the server runs with `--mode code`, *Show code*
    (`bec_code_traceback`). Servers without the analyser fall back to `bec_logs`.
  - *Queue and scans*: the queue every 5 s and the last scans. *Resume*, *Halt* and *Abort* appear
    only when a configured server offers `bec_queue_control` (a DEV-mode server), and each asks
    first.
- **Scan approvals**: when OpenCode asks before a BEC write tool runs, the GUI shows an approval
  card. OpenCode asks only where a permission rule says so, and bob's image ships none, so add one
  to OpenCode's configuration, for example `"permission": {"*bec_run_scan": "ask"}`. For
  `bec_run_scan` the card shows the call as you would type it, `scans.line_scan(dev.samx, -1, 1,
  steps=10, relative=True)`, and you can edit it. *Allow once* runs the agent's call; *Run edited
  scan* runs your version on the same server from the GUI and tells the agent what ran. Edited
  calls are parsed, never evaluated: only literals and `dev.<name>` are accepted.

To try scan approvals and queue controls without a beamline, run
[`web/scripts/fake_bec_dev_mcp.py`](web/scripts/fake_bec_dev_mcp.py) and add it to bob.toml as a second
server: it answers `bec_run_scan` and `bec_queue_control` like a DEV-mode server and only prints
what it was asked.

The panels take the server from OpenCode's configuration, rewriting `host.docker.internal` /
`host.containers.internal` to `127.0.0.1`, or from the URL set in the plugin's settings. MCP
servers send no CORS headers, so the requests go through a relay that accepts only loopback
targets: the dev/preview server's `/__bob/mcp` in the browser, a Rust command in the desktop app.
A server with `--token-file` needs its token in the plugin settings (kept for the window's
lifetime only).

## Writing a plugin

A plugin is a [`GuiPlugin`](web/src/plugins/types.ts) listed in
[`web/src/plugins/registry.ts`](web/src/plugins/registry.ts). It can describe tool calls (a label, a
summary and a body drawn under the row), describe permission requests, add side panels and
prompt suggestions, and add controls to its Settings row. Plugins only add views; the chat
works without them.

## Layout

| Path | What it holds |
| --- | --- |
| `web/src/api/` | OpenCode client (REST + event stream), connection parsing |
| `web/src/state/` | Store, event reducer, turn and usage arithmetic |
| `web/src/components/` | Chat UI, the changes panel and the todo card |
| `web/src/plugins/` | Plugin API and the BEC plugin |
| `web/server/bob-server.ts` | Vite plugin serving `/__bob/connection` and the MCP relay |
| `web/scripts/bob-gui.mjs` | Launcher: bob + GUI |
| `web/scripts/fake_bec_dev_mcp.py` | Stand-in DEV-mode BEC MCP server for trying write paths |
| `desktop/` | Desktop shell: the same commands as `web/server/`, in Rust |

## Checks

```sh
cd gui/web
npm run typecheck
npm test
npx vite build
(cd ../desktop && cargo fmt --check && cargo test)
```

Types of OpenCode's API come from `@opencode-ai/sdk`, pinned to the OpenCode version in
`image/Dockerfile`; update both together.
