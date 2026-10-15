import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { AssistantMessage, MessageEntry, Part, PermissionRequest, ToolPart } from "../api/types";
import { useEnabledPlugins } from "../plugins/registry";
import { errorText } from "../state/sessionData";
import { useStore } from "../state/store";
import { formatCost, formatDuration, formatTokens, groupTurns, turnStats, type Turn } from "../state/turns";
import { useNow } from "./hooks";
import { CopyButton } from "./CopyButton";
import { IconBrain, IconClock, IconDown, IconFork, IconTool, IconUndo } from "./Icons";
import { Markdown } from "./Markdown";
import { ThinkingBlock, ToolCall } from "./Parts";

function userText(entry: MessageEntry): string {
  return entry.parts
    .filter((part): part is Extract<Part, { type: "text" }> => part.type === "text" && !part.synthetic)
    .map((part) => part.text)
    .join("\n");
}

/** The sub-agent session a task tool call started, if OpenCode reported it. */
export function childSessionID(part: ToolPart): string | undefined {
  const meta = ("metadata" in part.state ? part.state.metadata : undefined) as Record<string, unknown> | undefined;
  const id = meta?.sessionId ?? meta?.sessionID ?? (part.metadata as Record<string, unknown> | undefined)?.sessionId;
  return typeof id === "string" ? id : undefined;
}

/** A sub-agent's own steps, loaded on demand and kept live by the event stream. */
function SubAgentRun({ sessionID }: { sessionID: string }) {
  const watch = useStore((s) => s.watchSession);
  const entries = useStore((s) => s.messages[sessionID]);
  const busy = useStore((s) => (s.status[sessionID]?.type ?? "idle") !== "idle");
  const permissions = useStore((s) => s.permissions);
  const mine = useMemo(() => Object.values(permissions).filter((p) => p.sessionID === sessionID), [permissions, sessionID]);
  useEffect(() => watch(sessionID), [watch, sessionID]);
  if (!entries) {
    return (
      <div className="working">
        <span className="spinner" /> Loading the sub-agent's work…
      </div>
    );
  }
  const steps = entries.filter((e): e is MessageEntry & { info: AssistantMessage } => e.info.role === "assistant");
  return (
    <div className="subagent">
      {steps.map((step, i) => (
        <StepParts key={step.info.id} step={step} live={busy && i === steps.length - 1} permissions={mine} />
      ))}
      {steps.length === 0 && <div className="muted">No steps yet.</div>}
    </div>
  );
}

const StepParts = memo(function StepParts({
  step,
  live,
  permissions,
}: {
  step: MessageEntry & { info: AssistantMessage };
  live: boolean;
  permissions: PermissionRequest[];
}) {
  const parts = step.parts.map((part) => {
        switch (part.type) {
          case "reasoning":
            return <ThinkingBlock key={part.id} part={part} live={live} />;
          case "text":
            return part.text.trim() ? <Markdown key={part.id} text={part.text} /> : null;
          case "compaction":
            return (
              <div key={part.id} className="divider">
                Chat compacted{part.auto ? " automatically" : ""}: what follows continues from a summary.
              </div>
            );
          case "tool": {
            const child = part.tool === "task" ? childSessionID(part) : undefined;
            return (
              <ToolCall
                key={part.id}
                part={part}
                live={live}
                permission={permissions.find((p) => p.tool?.callID === part.callID)}
                extra={child ? <SubAgentRun sessionID={child} /> : undefined}
              />
            );
          }
          case "patch":
            return (
              <button
                key={part.id}
                className="patch-link"
                title="Show the changes"
                onClick={() => useStore.getState().setPrefs({ panel: "core/changes" })}
              >
                Changed {part.files.length} {part.files.length === 1 ? "file" : "files"}:{" "}
                {part.files.map((file) => file.split("/").pop()).join(", ")}
              </button>
            );
          case "file":
            return (
              <div key={part.id} className="muted" style={{ fontSize: 12.5 }}>
                Attached {part.filename ?? part.url}
              </div>
            );
          case "retry":
            return (
              <div key={part.id} className="msg-error">
                Retrying: {errorText(part.error)}
              </div>
            );
          default:
            return null;
        }
      });
  return (
    <>
      {step.info.summary ? (
        <details className="summary-msg">
          <summary>Summary of the earlier chat</summary>
          {parts}
        </details>
      ) : (
        parts
      )}
      {step.info.error && step.info.error.name !== "MessageAbortedError" && (
        <div className="msg-error">{errorText(step.info.error)}</div>
      )}
      {step.info.error?.name === "MessageAbortedError" && <div className="muted">Stopped.</div>}
    </>
  );
});

function answerText(turn: Turn): string {
  return turn.steps
    .flatMap((step) => step.parts)
    .filter((part): part is Extract<Part, { type: "text" }> => part.type === "text" && !part.synthetic)
    .map((part) => part.text.trim())
    .filter(Boolean)
    .join("\n\n");
}

function TurnActions({ turn, nextID, busy }: { turn: Turn; nextID?: string; busy: boolean }) {
  const revert = useStore((s) => s.revert);
  const fork = useStore((s) => s.fork);
  const answer = answerText(turn);
  return (
    <span className="turn-actions">
      {answer && <CopyButton text={answer} />}
      <button className="icon-btn" title="Fork: a new chat with everything up to this answer" onClick={() => void fork(nextID)}>
        <IconFork size={14} />
      </button>
      {turn.user && (
        <button
          className="icon-btn"
          title="Undo this message and everything after it, including file changes"
          disabled={busy}
          onClick={() => void revert(turn.user!.info.id)}
        >
          <IconUndo size={14} />
        </button>
      )}
    </span>
  );
}

function TurnFooter({ turn, live, nextID, busy }: { turn: Turn; live: boolean; nextID?: string; busy: boolean }) {
  const now = useNow(live, 250);
  const stats = turnStats(turn, now);
  const thinkingOff = stats.variant ? /no[-_ ]?think/i.test(stats.variant) : false;
  return (
    <div className="turn-foot">
      {stats.modelID && (
        <span className="item" title={`${stats.providerID}/${stats.modelID}${stats.variant ? ` (${stats.variant})` : ""}`}>
          {stats.modelID}
          {thinkingOff ? " · no thinking" : stats.variant && stats.variant !== "default" ? ` · ${stats.variant}` : ""}
        </span>
      )}
      <span className="item" title="Time from the prompt to the last answer">
        <IconClock size={12} /> {formatDuration(stats.elapsedMs)}
      </span>
      {stats.tokens.input + stats.tokens.output > 0 && (
        <span
          className="item"
          title={`Input ${stats.tokens.input} · output ${stats.tokens.output} · reasoning ${stats.tokens.reasoning} · cache read ${stats.tokens.cacheRead} · cache write ${stats.tokens.cacheWrite}`}
        >
          {formatTokens(stats.tokens.input)} in · {formatTokens(stats.tokens.output)} out
        </span>
      )}
      {stats.tokens.reasoning > 0 && (
        <span className="item">
          <IconBrain size={12} /> {formatTokens(stats.tokens.reasoning)}
        </span>
      )}
      {stats.toolCalls > 0 && (
        <span className="item">
          <IconTool size={12} /> {stats.toolCalls}
        </span>
      )}
      {turn.steps.length > 1 && <span className="item">{turn.steps.length} steps</span>}
      {stats.cost > 0 && <span className="item">{formatCost(stats.cost)}</span>}
      {!live && <TurnActions turn={turn} nextID={nextID} busy={busy} />}
    </div>
  );
}

const TurnView = memo(function TurnView({
  turn,
  live,
  permissions,
  nextID,
  busy,
}: {
  turn: Turn;
  live: boolean;
  permissions: PermissionRequest[];
  /** First message of the next turn: forking "from here" copies everything before it. */
  nextID?: string;
  busy: boolean;
}) {
  const text = turn.user ? userText(turn.user) : "";
  const waitingForFirstStep = live && turn.steps.length === 0;
  const lastStep = turn.steps[turn.steps.length - 1];
  // Between steps, or before a step shows anything, nothing else says the assistant is busy.
  const stillWorking =
    live &&
    lastStep &&
    (Boolean(lastStep.info.time.completed) ||
      !lastStep.parts.some((p) => p.type === "text" || p.type === "reasoning" || p.type === "tool"));
  return (
    <div className="turn">
      {text && (
        <div className="user-msg">
          <div className="bubble">{text}</div>
        </div>
      )}
      <div className="assistant">
        {turn.steps.map((step, index) => (
          <StepParts key={step.info.id} step={step} live={live && index === turn.steps.length - 1} permissions={permissions} />
        ))}
        {(waitingForFirstStep || stillWorking) && (
          <div className="working">
            <span className="spinner" /> Working…
          </div>
        )}
        {(turn.steps.length > 0 || live) && <TurnFooter turn={turn} live={live} nextID={nextID} busy={busy} />}
      </div>
    </div>
  );
});

function EmptyChat() {
  const plugins = useEnabledPlugins();
  const send = useStore((s) => s.send);
  const suggestions = useMemo(() => {
    const list = plugins.flatMap((p) => p.suggestions ?? []);
    return list.length ? list : ["What can you do?", "Summarise the files in the workspace", "Explain the project layout"];
  }, [plugins]);
  return (
    <div className="empty-chat">
      <h2>What can I help with?</h2>
      <div className="muted">The assistant runs in bob's container, with its tools and MCP servers.</div>
      <div className="suggestions">
        {suggestions.map((s) => (
          <button key={s} className="suggestion" onClick={() => void send(s)}>
            {s}
          </button>
        ))}
      </div>
    </div>
  );
}

export function Thread() {
  const currentId = useStore((s) => s.currentId);
  const entries = useStore((s) => (s.currentId ? s.messages[s.currentId] : undefined));
  const loading = useStore((s) => (s.currentId ? s.loading[s.currentId] : false));
  const busy = useStore((s) => (s.currentId ? s.status[s.currentId]?.type !== undefined && s.status[s.currentId]?.type !== "idle" : false));
  const retry = useStore((s) => (s.currentId ? s.status[s.currentId] : undefined));
  const sessionError = useStore((s) => (s.currentId ? s.errors[s.currentId] : undefined));
  const allPermissions = useStore((s) => s.permissions);
  const permissions = useMemo(
    () => Object.values(allPermissions).filter((p) => p.sessionID === currentId),
    [allPermissions, currentId],
  );
  const turns = useMemo(() => groupTurns(entries ?? []), [entries]);
  const revertID = useStore((s) => (s.currentId ? s.sessions[s.currentId]?.revert?.messageID : undefined));
  const unrevert = useStore((s) => s.unrevert);
  // The session error repeats the failed step's own error; show it only when no step carries it.
  const lastSteps = turns[turns.length - 1]?.steps;
  const lastStepFailed = Boolean(lastSteps?.[lastSteps.length - 1]?.info.error);

  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const [showJump, setShowJump] = useState(false);

  useEffect(() => {
    pinned.current = true;
    setShowJump(false);
  }, [currentId]);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && pinned.current) el.scrollTop = el.scrollHeight;
  });

  // Content also grows without a render here (a card opening, the panel narrowing the column):
  // stay at the bottom then too, unless the user scrolled up.
  const inner = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = scroller.current;
    const content = inner.current;
    if (!el || !content) return;
    const follow = () => {
      if (pinned.current) el.scrollTop = el.scrollHeight;
    };
    const observer = new ResizeObserver(follow);
    observer.observe(el);
    observer.observe(content);
    return () => observer.disconnect();
  }, [currentId]);

  if (!currentId) {
    return (
      <div className="thread">
        <div className="thread-inner">
          <EmptyChat />
        </div>
      </div>
    );
  }

  return (
    <div
      className="thread"
      ref={scroller}
      onScroll={(event) => {
        const el = event.currentTarget;
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        pinned.current = atBottom;
        setShowJump(!atBottom);
      }}
    >
      <div className="thread-inner" ref={inner}>
        {loading && !entries && (
          <div className="working">
            <span className="spinner" /> Loading the chat…
          </div>
        )}
        {entries && turns.length === 0 && !busy && <EmptyChat />}
        {turns.map((turn, index) => (
          <div key={turn.id} className={revertID && turn.id >= revertID ? "undone" : undefined}>
            {revertID && turn.id >= revertID && (index === 0 || turns[index - 1].id < revertID) && (
              <div className="undo-banner">
                <IconUndo size={14} />
                <span className="grow">
                  Undone from here, workspace files included. Your next message replaces it.
                </span>
                <button className="btn small" onClick={() => void unrevert()}>
                  Redo
                </button>
              </div>
            )}
            <TurnView
              turn={turn}
              live={busy && index === turns.length - 1}
              permissions={permissions}
              nextID={turns[index + 1]?.id}
              busy={busy}
            />
          </div>
        ))}
        {retry?.type === "retry" && (
          <div className="msg-error">
            Retrying (attempt {retry.attempt}): {retry.message}
          </div>
        )}
        {sessionError && !busy && !lastStepFailed && <div className="msg-error">{sessionError}</div>}
      </div>
      {showJump && (
        <div className="jump">
          <button
            className="btn small"
            onClick={() => {
              pinned.current = true;
              scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });
            }}
          >
            <IconDown size={14} /> Latest
          </button>
        </div>
      )}
    </div>
  );
}
