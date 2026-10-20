import { memo, useState, type ReactNode } from "react";
import type { PermissionRequest, ReasoningPart, ToolPart } from "../api/types";
import { formatDuration } from "../state/turns";
import { useNow } from "./hooks";
import { IconBrain, IconCheck, IconChevron, IconShield, IconX } from "./Icons";

export const ThinkingBlock = memo(function ThinkingBlock({ part, live }: { part: ReasoningPart; live: boolean }) {
  const running = live && !part.time.end;
  const now = useNow(running, 200);
  const [open, setOpen] = useState<boolean | null>(null);
  const text = part.text.trim();
  if (!text && !running) return null;
  // Reasoning cut off by a stop has no end; showing "now - start" would grow forever.
  const elapsed = part.time.end ? part.time.end - part.time.start : running ? now - part.time.start : undefined;
  const expanded = open ?? running;
  return (
    <details className="thinking" open={expanded}>
      <summary
        className="thinking-head"
        onClick={(event) => {
          event.preventDefault();
          setOpen(!expanded);
        }}
      >
        <IconChevron size={13} className="chev" />
        <IconBrain size={14} />
        {running ? (
          <span>Thinking… {formatDuration(elapsed ?? 0)}</span>
        ) : (
          <span>{elapsed === undefined ? "Thought" : `Thought for ${formatDuration(elapsed)}`}</span>
        )}
        {running && <span className="dot pulse" style={{ background: "var(--think)" }} />}
      </summary>
      <div className="thinking-body">{text || "…"}</div>
    </details>
  );
});

function humanize(tool: string): string {
  return tool.replace(/[_-]+/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

function pretty(value: unknown): string {
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      try {
        return JSON.stringify(JSON.parse(trimmed), null, 2);
      } catch {
        return value;
      }
    }
    return value;
  }
  return JSON.stringify(value, null, 2);
}

/** A label and summary for the row, where the raw call says too little: a sub-agent's name and task. */
function toolView(part: ToolPart): { label?: string; summary?: string } {
  if (part.tool !== "task") return {};
  const input = part.state.input;
  return {
    label: `Sub-agent${typeof input.subagent_type === "string" ? ` · ${input.subagent_type}` : ""}`,
    summary: typeof input.description === "string" ? input.description : undefined,
  };
}

const OUTPUT_LIMIT = 20_000;

export const ToolCall = memo(function ToolCall({
  part,
  live,
  permission,
  extra,
}: {
  part: ToolPart;
  live: boolean;
  permission?: PermissionRequest;
  /** Drawn first when the row is open, e.g. a sub-agent's own steps. */
  extra?: ReactNode;
}) {
  const view = toolView(part);
  const state = part.state;
  const running = live && (state.status === "pending" || state.status === "running");
  const now = useNow(running, 200);
  const [open, setOpen] = useState<boolean | null>(null);

  const start = "time" in state && state.time ? state.time.start : undefined;
  const end = state.status === "completed" || state.status === "error" ? state.time.end : undefined;
  const elapsed = start && (end || running) ? (end ?? now) - start : undefined;
  const title = (state.status === "completed" || state.status === "running") && state.title ? state.title : "";
  const summary =
    view.summary ?? (state.status === "error" ? state.error : title && title !== part.tool ? title : "");
  const expanded = open ?? (Boolean(permission) || (Boolean(extra) && running));

  const status =
    permission ? (
      <IconShield size={15} className="tone-warn" />
    ) : running ? (
      <span className="spinner" />
    ) : state.status === "completed" ? (
      <IconCheck size={15} className="tone-ok" />
    ) : state.status === "error" ? (
      <IconX size={15} className="tone-error" />
    ) : (
      <span className="dot" />
    );

  const output = state.status === "completed" ? state.output : state.status === "error" ? state.error : "";
  return (
    <details className="tool" open={expanded}>
      <summary
        className="tool-head"
        onClick={(event) => {
          event.preventDefault();
          setOpen(!expanded);
        }}
      >
        <span className="tool-status">{status}</span>
        <span className="name" title={part.tool}>
          {view.label ?? humanize(part.tool)}
        </span>
        <span className={`summary ${state.status === "error" ? "tone-error" : ""}`}>
          {permission ? "Waiting for your approval" : summary}
        </span>
        {elapsed !== undefined && <span className="time">{formatDuration(elapsed)}</span>}
        <IconChevron size={13} className="chev" />
      </summary>
      {expanded && extra && <div className="tool-rich">{extra}</div>}
      {expanded && (
        <div className="tool-body">
          <h5>Input</h5>
          <pre>{pretty(state.input)}</pre>
          {output && (
            <>
              <h5>{state.status === "error" ? "Error" : "Output"}</h5>
              <pre>
                {output.length > OUTPUT_LIMIT ? `${pretty(output.slice(0, OUTPUT_LIMIT))}\n…` : pretty(output)}
              </pre>
            </>
          )}
        </div>
      )}
    </details>
  );
});
