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

The GUI is a [Tauri](https://tauri.app) desktop app: a native window around the React page in
`web/`. The page talks to OpenCode directly; the app's Rust side (`desktop/`) starts and holds
bob and does what a page cannot. Opened in a browser, the page only says so.

## Run it

Requires Node 20+, Go (to build bob), a Rust toolchain and the
[Tauri prerequisites](https://tauri.app/start/prerequisites/) for your platform (Xcode command
line tools on macOS; WebKitGTK on Linux). From the repository root:

```sh
npm --prefix gui/web install
npm --prefix gui/web run bob          # builds bob into build/bob-manager, then runs the app (tauri dev)
npm --prefix gui/web run tauri build  # a bundle in gui/desktop/target/release/bundle
```

The app uses your bob (see [how bob
runs](../launcher/container_manager/README.md#how-bob-runs)): as soon as it opens, before the
page has loaded, it joins the bob you already run, or starts it with your bob.toml when none
runs, unless *Do this when the app opens* is off. It holds bob until you quit it or choose
*Leave this bob* in Settings; bob stops 90 seconds after the last program that uses it (this
app, BEC widgets) is gone. A crash ends the hold too. The app holds one bob at most: its Rust
side keeps the hold and tells the page about every change. If bob stops while the app holds it
(its container died, or its broker was stopped), a bar above the chat says so and offers to join
again. When bob already runs with another bob.toml, started by another program, the app uses
that bob and says so.

The connect screen sets the bob.toml and the bob executable. The executable defaults to
`$BOB_BIN`, then `build/bob-manager` of the source tree the app was built from (`npm run bob`
builds it), then `bob` on the PATH; the bob.toml defaults to the one at the root of that source
tree. The app keeps these choices in its own settings file, `settings.json` in its config folder
(`~/Library/Application Support/ch.psi.bob` on macOS, `~/.config/ch.psi.bob` on Linux), so it
can start bob before the page loads; the page keeps only look and layout. The screen also lists
every running bob container on Docker or Podman; connecting to one of those does not hold it. To
read such a container's OpenCode password the app uses `docker inspect`, which only someone who
can already control the engine can do. *Connect by hand…* takes a connection (URL and password)
of a bob running elsewhere. Settings → *Leave this bob* (or *Switch bob…*, when the app holds
none) goes back to that screen.

## Thinking

OpenCode switches thinking off through a model *variant*. bob's image defines one for the
self-hosted model in [`containers/opencode.json`](../containers/opencode.json):

```json
"variants": { "no-thinking": { "chat_template_kwargs": { "enable_thinking": false } } }
```

`chat_template_kwargs` is what vLLM-served Qwen models honour (PSI's gateway included). The
switch is greyed out for a model without a variant whose name contains `no-thinking`; rebuild
the image (`docker build -t localhost/bob ./containers`) to get it.

## Context

bob tells OpenCode the model's context window when `provider.context_limit` is set in bob.toml
(PSI's gateway reports 262144 tokens for Qwen3.8-27B). The usage bar then shows how full the
context of the last model call was, and OpenCode compacts a chat on its own before it overflows.
*Compact* does it on demand: the chat continues from a summary, shown as *Summary of the earlier
chat*.

## Layout

| Path | What it holds |
| --- | --- |
| `web/src/api/` | OpenCode client (REST + event stream), connection parsing |
| `web/src/state/` | Store, event reducer, turn and usage arithmetic |
| `web/src/components/` | Chat UI, the changes panel and the todo card |
| `web/src/platform.ts` | The page's calls into the desktop app |
| `desktop/` | The desktop app's Rust side: the lease on bob (`src/bob.rs`), its settings (`src/settings.rs`), links and downloads |

## Checks

```sh
cd gui/web
npm run typecheck
npm test
npx vite build
(cd ../desktop && cargo fmt --check && cargo test)
```

Types of OpenCode's API come from `@opencode-ai/sdk`, pinned to the OpenCode version in
`containers/Dockerfile`; update both together.
