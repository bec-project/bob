# BEC plugin

Report cards, panels and scan approvals for the BEC MCP server, as a plugin of bob's Qt app (see
[Plugins](../../../../README.md#plugins)).

Enable it for a BEC MCP server in bob.toml, with any server name:

```toml
[mcp.bec_mcp]
url = "http://host.docker.internal:8000/mcp"
plugin = "bec"
```

Each such server gets its own instance: its tool calls get cards, its panels read that server
only, and Settings → Plugins has a switch for it ("BEC · bec_mcp").

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
    only when this server offers `bec_queue_control` (a DEV-mode server), and each asks first.
- **Scan approvals**: when OpenCode asks before a BEC write tool runs, the app shows an approval
  card. OpenCode asks only where a permission rule says so, and bob's image ships none, so add one
  to OpenCode's configuration, for example `"permission": {"*bec_run_scan": "ask"}`. For
  `bec_run_scan` the card shows the call as you would type it, `scans.line_scan(dev.samx, -1, 1,
  relative=True, steps=10)`, keyword arguments in alphabetical order; *Allow once* runs exactly
  that. The card cannot edit the call: a write from the app next to the agent's own could run a
  scan twice, so changed scans wait for approvals that the MCP server itself enforces.

To try scan approvals and queue controls without a beamline, run
[`fake_bec_dev_mcp.py`](fake_bec_dev_mcp.py) and add it to bob.toml as a second server with
`plugin = "bec"`: it answers `bec_run_scan` and `bec_queue_control` like a DEV-mode server and
only prints what it was asked.

A server started with `--token-file` needs its token in the instance's settings (kept for the
window's lifetime only).

| File | What it holds |
| --- | --- |
| `BecPlugin.qml` | The plugin: tool rows, the scan approval, panels, suggestions, settings |
| `beccards.cpp`, `beccards.h` | `BecCards` (C++): report cards built from the BEC tools' results, tool rows, and scan calls as typed in the BEC client |
| `becserver.cpp`, `becserver.h` | `BecServer` (C++): calls to the instance's server, and its tool list |
| `BecCard.qml`, `BecIncidents.qml`, `BecLogRecords.qml`, `BecTraceback.qml`, `BecCodeLines.qml` | The views that draw the cards |
| `BecDevicesPanel.qml`, `BecIncidentsPanel.qml`, `BecQueuePanel.qml` | The panels |
| `designer/` | Stand-ins for `BecCards` and `BecServer`, for Qt Design Studio (named in `qmldir`) |
| `fixtures/` | Recorded tool results for the card tests (`gui/qt/tests/tst_bec.cpp`) |
| `fake_bec_dev_mcp.py` | Stand-in DEV-mode BEC MCP server for trying write paths |
