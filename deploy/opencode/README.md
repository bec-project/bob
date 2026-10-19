# Example OpenCode configuration

Two demo agents that show what a profile can set: its purpose (the prompt), which tools it
has, and whether each tool runs, asks first or is switched off. Point bob at this directory
with `opencode.config_dir` in `bob.toml`; OpenCode then lists both agents next to its built-in
`build` and `plan`, and a client picks one per session or message.

| Agent | Purpose | Tools |
|---|---|---|
| `inspect` | Investigate and explain without changing anything | Reads, searches and read-only `git` run; everything else, MCP tool calls included, is off |
| `operator` | Carry out tasks with the connected MCP tools | Every MCP tool call asks first; file edits, shell and web access are off |

Each agent is a Markdown file in `agents/`: the front matter holds the settings and the body is
the prompt. Permission rules are checked in order and the last match wins. Keys are tool names
(`read`, `edit`, `bash`, `webfetch`, …) or MCP tool names, which OpenCode prefixes with the
server's name from `bob.toml` (`bec_mcp_bec_run_scan` for `[mcp.bec_mcp]`), so `"*bec_run_scan"`
matches that tool on any server. A `bash` rule can match single commands.

The directory can also hold `commands/*.md` and an `opencode.json`, for example with permission
rules for all agents. Keep the `.gitignore`: OpenCode writes one at startup unless it exists, and
bob mounts the directory read-only.

OpenCode's built-in tools that list MCP resources ignore permission rules (checked with OpenCode
1.18.35), so even `inspect` can list what a server offers as resources; MCP tool calls follow
the rules.

These rules steer the model; they are not a security boundary. Tools the agent runs can reach
the configured MCP endpoints directly, so hard limits belong in the MCP server.
