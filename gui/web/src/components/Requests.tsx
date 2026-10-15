import { useMemo, useState } from "react";
import type { MessageEntry, PermissionRequest, QuestionRequest, ToolPart } from "../api/types";
import { permissionView, useEnabledPlugins } from "../plugins/registry";
import { useStore } from "../state/store";
import { IconQuestion, IconShield } from "./Icons";

function findTool(entries: MessageEntry[] | undefined, callID: string | undefined): ToolPart | undefined {
  if (!entries || !callID) return undefined;
  for (let i = entries.length - 1; i >= 0; i--) {
    const part = entries[i].parts.find((p): p is ToolPart => p.type === "tool" && p.callID === callID);
    if (part) return part;
  }
  return undefined;
}

function describe(request: PermissionRequest, tool: ToolPart | undefined): { title: string; detail: string } {
  const meta = request.metadata as Record<string, unknown>;
  const patterns = request.patterns.filter((p) => p !== "*");
  switch (request.permission) {
    case "bash":
      return { title: "Run this command?", detail: String(meta.command ?? patterns.join("\n")) };
    case "edit":
      return { title: "Change this file?", detail: String(meta.filepath ?? patterns.join("\n")) + (meta.diff ? `\n\n${meta.diff}` : "") };
    case "external_directory":
      return { title: "Work outside the workspace?", detail: patterns.join("\n") };
    case "webfetch":
      return { title: "Fetch this page?", detail: String(meta.url ?? patterns.join("\n")) };
    case "doom_loop":
      return { title: "The assistant repeats the same call. Let it continue?", detail: tool ? JSON.stringify(tool.state.input, null, 2) : "" };
    default: {
      const input = tool ? JSON.stringify(tool.state.input, null, 2) : "";
      return {
        title: `Allow ${request.permission}?`,
        detail: [patterns.join("\n"), input].filter(Boolean).join("\n\n"),
      };
    }
  }
}

function PermissionCard({ request, tool }: { request: PermissionRequest; tool?: ToolPart }) {
  const reply = useStore((s) => s.replyPermission);
  const enabled = useEnabledPlugins().map((p) => p.id);
  const [busy, setBusy] = useState(false);
  const custom = permissionView(request, tool, enabled);
  const base = describe(request, tool);
  const answer = async (value: "once" | "always" | "reject", message?: string) => {
    setBusy(true);
    await reply(request.id, value, message);
    setBusy(false);
  };
  const body = typeof custom?.body === "function" ? custom.body({ reply: answer }) : custom?.body;
  return (
    <div className="request" onKeyDown={(event) => {
      if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) void answer("once");
    }}>
      <h4>
        <IconShield size={16} className="tone-warn" />
        {custom?.title ?? base.title}
      </h4>
      <div className="muted" style={{ fontSize: 12.5 }}>
        {tool ? <>Tool <code>{tool.tool}</code></> : <>Permission <code>{request.permission}</code></>}
      </div>
      {body ?? (base.detail ? <pre>{base.detail}</pre> : null)}
      <div className="row">
        <button className="btn primary" autoFocus disabled={busy} onClick={() => void answer("once")}>
          Allow once
        </button>
        {request.always.length > 0 && (
          <button className="btn" disabled={busy} onClick={() => void answer("always")} title={`Always allow: ${request.always.join(", ")}`}>
            Always allow
          </button>
        )}
        <button className="btn danger" disabled={busy} onClick={() => void answer("reject")}>
          Reject
        </button>
      </div>
    </div>
  );
}

function QuestionCard({ request }: { request: QuestionRequest }) {
  const reply = useStore((s) => s.replyQuestion);
  const reject = useStore((s) => s.rejectQuestion);
  const [answers, setAnswers] = useState<string[][]>(() => request.questions.map(() => []));
  const [custom, setCustom] = useState<string[]>(() => request.questions.map(() => ""));
  const complete = request.questions.every((_, i) => answers[i].length > 0 || custom[i].trim());
  return (
    <div className="request">
      {request.questions.map((question, qi) => (
        <div key={qi} style={{ marginBottom: 10 }}>
          <h4>
            <IconQuestion size={16} className="tone-info" />
            {question.question}
          </h4>
          <div className="question-options">
            {question.options.map((option) => {
              const selected = answers[qi].includes(option.label);
              return (
                <div
                  key={option.label}
                  className={`question-option ${selected ? "selected" : ""}`}
                  onClick={() =>
                    setAnswers((prev) =>
                      prev.map((list, i) =>
                        i !== qi
                          ? list
                          : question.multiple
                            ? selected
                              ? list.filter((l) => l !== option.label)
                              : [...list, option.label]
                            : [option.label],
                      ),
                    )
                  }
                >
                  <input type={question.multiple ? "checkbox" : "radio"} readOnly checked={selected} />
                  <div>
                    {option.label}
                    {option.description && <small>{option.description}</small>}
                  </div>
                </div>
              );
            })}
            {question.custom !== false && (
              <input
                className="input"
                placeholder="Or type your own answer"
                value={custom[qi]}
                onChange={(event) => setCustom((prev) => prev.map((v, i) => (i === qi ? event.target.value : v)))}
              />
            )}
          </div>
        </div>
      ))}
      <div className="row">
        <button
          className="btn primary"
          disabled={!complete}
          onClick={() =>
            void reply(
              request.id,
              answers.map((list, i) => (custom[i].trim() ? [...list, custom[i].trim()] : list)),
            )
          }
        >
          Answer
        </button>
        <button className="btn" onClick={() => void reject(request.id)}>
          Dismiss
        </button>
      </div>
    </div>
  );
}

/** Permission and question requests of the open chat, shown above the composer. */
export function Requests() {
  const currentId = useStore((s) => s.currentId);
  const permissions = useStore((s) => s.permissions);
  const questions = useStore((s) => s.questions);
  const entries = useStore((s) => (s.currentId ? s.messages[s.currentId] : undefined));
  const mine = useMemo(
    () => ({
      permissions: Object.values(permissions).filter((p) => p.sessionID === currentId),
      questions: Object.values(questions).filter((q) => q.sessionID === currentId),
    }),
    [permissions, questions, currentId],
  );
  if (!mine.permissions.length && !mine.questions.length) return null;
  return (
    <div className="requests">
      {mine.permissions.map((request) => (
        <PermissionCard key={request.id} request={request} tool={findTool(entries, request.tool?.callID)} />
      ))}
      {mine.questions.map((request) => (
        <QuestionCard key={request.id} request={request} />
      ))}
    </div>
  );
}
