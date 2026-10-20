import { useMemo } from "react";
import type { ModelRef } from "../api/types";
import { activeModel, findModel, noThinkingVariant, useStore } from "../state/store";
import { addTokens, formatCost, formatDuration, formatTokens, groupTurns, noTokens, turnStats } from "../state/turns";
import { useNow } from "./hooks";
import { saveTextFile } from "../platform";
import { chatMarkdown, exportFileName } from "../state/exportMarkdown";
import { IconBrain, IconCompress, IconCopy, IconDownload, IconFork, IconSidebar } from "./Icons";

const modelKey = (ref?: ModelRef) => (ref ? `${ref.providerID}/${ref.modelID}` : "");

function ModelPicker() {
  const providers = useStore((s) => s.providers);
  const model = useStore((s) => activeModel(s));
  const setPrefs = useStore((s) => s.setPrefs);
  const options = useMemo(
    () =>
      providers.flatMap((provider) =>
        Object.values(provider.models).map((m) => ({
          key: `${provider.id}/${m.id}`,
          label: providers.length > 1 ? `${provider.name} · ${m.name}` : m.name,
          ref: { providerID: provider.id, modelID: m.id },
        })),
      ),
    [providers],
  );
  if (options.length <= 1) {
    return (
      <span className="chip" title={modelKey(model)}>
        {options[0]?.label ?? model?.modelID ?? "no model"}
      </span>
    );
  }
  return (
    <select
      className="select"
      title="Model for the next message"
      value={modelKey(model)}
      onChange={(event) => setPrefs({ model: options.find((o) => o.key === event.target.value)?.ref })}
    >
      {options.map((option) => (
        <option key={option.key} value={option.key}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

function ThinkingToggle() {
  const thinking = useStore((s) => s.prefs.thinking);
  const setPrefs = useStore((s) => s.setPrefs);
  const variant = useStore((s) => noThinkingVariant(findModel(s.providers, activeModel(s))));
  const disabled = !variant;
  return (
    <label
      className={`toggle ${disabled ? "disabled" : ""}`}
      title={
        disabled
          ? "This model has no no-thinking variant in OpenCode's configuration. Rebuild bob's image to add it."
          : thinking
            ? "The model reasons before answering. Switch off for faster, shorter answers."
            : `Thinking is off (variant “${variant}”).`
      }
    >
      <IconBrain size={15} />
      Thinking
      <input
        type="checkbox"
        checked={disabled ? true : thinking}
        disabled={disabled}
        onChange={(event) => setPrefs({ thinking: event.target.checked })}
      />
      <span className="track" />
    </label>
  );
}

function AgentPicker() {
  const agents = useStore((s) => s.agents);
  const agent = useStore((s) => s.prefs.agent ?? s.defaultAgent);
  const setPrefs = useStore((s) => s.setPrefs);
  const primary = agents.filter((a) => a.mode !== "subagent" && !a.hidden);
  if (primary.length <= 1) return null;
  return (
    <select
      className="select"
      title="Agent: build can change files, plan only reads"
      value={agent}
      onChange={(event) => setPrefs({ agent: event.target.value })}
    >
      {primary.map((a) => (
        <option key={a.name} value={a.name}>
          {a.name}
        </option>
      ))}
    </select>
  );
}

function ChatTools() {
  const session = useStore((s) => (s.currentId ? s.sessions[s.currentId] : undefined));
  const hasMessages = useStore((s) => Boolean(s.currentId && s.messages[s.currentId]?.length));
  const fork = useStore((s) => s.fork);
  const setNotice = useStore((s) => s.setNotice);
  if (!session || !hasMessages) return null;
  const markdown = () => {
    const state = useStore.getState();
    return chatMarkdown(session, state.messages[session.id] ?? []);
  };
  return (
    <span className="chat-tools">
      <button className="icon-btn" title="Fork: a copy of this chat to continue in a new direction" onClick={() => void fork()}>
        <IconFork size={16} />
      </button>
      <button
        className="icon-btn"
        title="Copy the chat as Markdown"
        onClick={() => void navigator.clipboard?.writeText(markdown()).then(() => setNotice("Copied the chat as Markdown."))}
      >
        <IconCopy size={15} />
      </button>
      <button
        className="icon-btn"
        title="Export the chat as a Markdown file"
        onClick={() =>
          void saveTextFile(exportFileName(session), markdown())
            .then((path) => path && setNotice(`Saved the chat to ${path}.`))
            .catch((error: Error) => setNotice(`Could not save the chat: ${error.message}`))
        }
      >
        <IconDownload size={16} />
      </button>
    </span>
  );
}

const MCP_STATES: Record<string, { tone: string; text: string }> = {
  connected: { tone: "ok", text: "connected" },
  connecting: { tone: "warn pulse", text: "connecting…" },
  disabled: { tone: "", text: "switched off in OpenCode's configuration" },
  failed: { tone: "error", text: "cannot connect" },
  needs_auth: { tone: "warn", text: "needs authorization" },
  needs_client_registration: { tone: "warn", text: "needs client registration" },
};

/**
 * One chip per MCP server OpenCode uses (bob.toml's [mcp.*]), coloured by its state. A chip of a
 * server that is not connected connects it again when clicked.
 */
function McpStatusChips() {
  const status = useStore((s) => s.mcpStatus);
  const reconnect = useStore((s) => s.reconnectMcp);
  const names = Object.keys(status).sort();
  if (names.length === 0) return null;
  return (
    <div className="mcp-chips">
      {names.map((name) => {
        const entry = status[name];
        const known = MCP_STATES[entry.status] ?? { tone: "warn", text: entry.status };
        const retry = entry.status !== "connected" && entry.status !== "connecting" && entry.status !== "disabled";
        const detail = `MCP server ${name}: ${known.text}${entry.error ? ` (${entry.error})` : ""}`;
        return (
          <button
            key={name}
            className={`mcp-chip ${known.tone.split(" ")[0]}`}
            title={retry ? `${detail}. Click to connect again.` : detail}
            disabled={!retry}
            onClick={() => void reconnect(name)}
          >
            <span className={`dot ${known.tone}`} />
            {name}
          </button>
        );
      })}
    </div>
  );
}

export function Header() {
  const session = useStore((s) => (s.currentId ? s.sessions[s.currentId] : undefined));
  const sidebar = useStore((s) => s.prefs.sidebar);
  const setPrefs = useStore((s) => s.setPrefs);
  return (
    <header className="header">
      <button className="icon-btn" title="Toggle chat list" onClick={() => setPrefs({ sidebar: !sidebar })}>
        <IconSidebar size={17} />
      </button>
      <div className="title">{session ? session.title || "Untitled" : "New chat"}</div>
      <div className="controls">
        <McpStatusChips />
        <ChatTools />
        <AgentPicker />
        <ModelPicker />
        <ThinkingToggle />
      </div>
    </header>
  );
}

/** Session totals: tokens, time spent answering, cost when the provider reports one. */
export function UsageBar() {
  const currentId = useStore((s) => s.currentId);
  const entries = useStore((s) => (s.currentId ? s.messages[s.currentId] : undefined));
  const busy = useStore((s) => (s.currentId ? s.status[s.currentId]?.type === "busy" : false));
  const providers = useStore((s) => s.providers);
  const compact = useStore((s) => s.compact);
  const compacting = useStore((s) => (s.currentId ? Boolean(s.sessions[s.currentId]?.time.compacting) : false));
  const now = useNow(busy, 500);
  const stats = useMemo(() => {
    const turns = groupTurns(entries ?? []);
    let tokens = noTokens;
    let cost = 0;
    let time = 0;
    let tools = 0;
    let last;
    for (const turn of turns) {
      const s = turnStats(turn, now);
      tokens = addTokens(tokens, s.tokens);
      cost += s.cost;
      time += s.elapsedMs;
      tools += s.toolCalls;
      // a stopped or running turn has no token count yet: keep the last one that has
      if (s.contextTokens > 0) last = s;
    }
    return { turns: turns.length, tokens, cost, time, tools, last };
  }, [entries, now]);
  if (!currentId || !entries || stats.turns === 0) return null;
  const model = stats.last?.providerID
    ? findModel(providers, { providerID: stats.last.providerID, modelID: stats.last.modelID ?? "" })
    : undefined;
  const limit = model?.limit?.context ?? 0;
  const context = stats.last?.contextTokens ?? 0;
  return (
    <div className="usage" title="Totals for this chat">
      <span>
        <b>{stats.turns}</b> {stats.turns === 1 ? "message" : "messages"}
      </span>
      <span className="sep" />
      <span>
        <b>{formatDuration(stats.time)}</b> answering
      </span>
      <span className="sep" />
      <span title="Input tokens sent to the model, summed over every step">
        in <b>{formatTokens(stats.tokens.input)}</b>
      </span>
      <span>
        out <b>{formatTokens(stats.tokens.output)}</b>
      </span>
      {stats.tokens.reasoning > 0 && (
        <span>
          thinking <b>{formatTokens(stats.tokens.reasoning)}</b>
        </span>
      )}
      {stats.tokens.cacheRead > 0 && (
        <span>
          cached <b>{formatTokens(stats.tokens.cacheRead)}</b>
        </span>
      )}
      <span className="sep" />
      <span
        className="context-meter"
        title={
          limit > 0
            ? `The last model call used ${context} of ${limit} context tokens. OpenCode compacts the chat on its own when it fills up.`
            : "Tokens in the context of the last model call. Set provider.context_limit in bob.toml to see how full it is."
        }
      >
        context <b>{formatTokens(context)}</b>
        {limit > 0 && (
          <>
            {` / ${formatTokens(limit)}`}
            <span className="meter">
              <span
                className={`fill ${context / limit >= 0.8 ? "warn" : ""}`}
                style={{ width: `${Math.min(100, (context / limit) * 100)}%` }}
              />
            </span>
            {`${Math.round((context / limit) * 100)}%`}
          </>
        )}
      </span>
      {stats.tools > 0 && (
        <>
          <span className="sep" />
          <span>
            <b>{stats.tools}</b> tool {stats.tools === 1 ? "call" : "calls"}
          </span>
        </>
      )}
      {stats.cost > 0 && (
        <>
          <span className="sep" />
          <span>
            <b>{formatCost(stats.cost)}</b>
          </span>
        </>
      )}
      <button
        className="btn small ghost"
        title="Summarise the chat so far; later messages start from the summary"
        disabled={busy || compacting}
        onClick={() => void compact()}
      >
        <IconCompress size={12} /> Compact
      </button>
    </div>
  );
}
