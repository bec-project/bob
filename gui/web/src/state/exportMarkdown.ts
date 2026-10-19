import type { MessageEntry, Part, Session } from "../api/types";
import { formatDuration, formatTokens, groupTurns, turnStats } from "./turns";

function text(parts: Part[]): string {
  return parts
    .filter((part): part is Extract<Part, { type: "text" }> => part.type === "text" && !part.synthetic)
    .map((part) => part.text.trim())
    .filter(Boolean)
    .join("\n\n");
}

function toolLine(part: Extract<Part, { type: "tool" }>): string {
  const state = part.state;
  const title = "title" in state && state.title ? state.title : "";
  const status = state.status === "error" ? ` (failed: ${state.error.split("\n")[0]})` : "";
  const input = JSON.stringify(state.input);
  return `- \`${part.tool}\`${title ? ` ${title}` : ""}${status}${input && input !== "{}" ? ` \`${input.length > 200 ? `${input.slice(0, 200)}…` : input}\`` : ""}`;
}

/** The chat as Markdown: prompts, answers, the tools each answer used, and its usage. */
export function chatMarkdown(session: Session | undefined, entries: MessageEntry[], options: { thinking?: boolean } = {}): string {
  const lines: string[] = [`# ${session?.title || "Chat"}`, ""];
  if (session) lines.push(`Exported ${new Date().toISOString().slice(0, 16).replace("T", " ")} · started ${new Date(session.time.created).toISOString().slice(0, 16).replace("T", " ")}`, "");
  for (const turn of groupTurns(entries)) {
    if (turn.user) {
      lines.push("## You", "", text(turn.user.parts), "");
    }
    const tools = turn.steps.flatMap((step) => step.parts.filter((p): p is Extract<Part, { type: "tool" }> => p.type === "tool"));
    const answer = turn.steps.map((step) => text(step.parts)).filter(Boolean).join("\n\n");
    const thinking = options.thinking
      ? turn.steps
          .flatMap((step) => step.parts)
          .filter((p): p is Extract<Part, { type: "reasoning" }> => p.type === "reasoning" && Boolean(p.text.trim()))
          .map((p) => p.text.trim())
      : [];
    if (!turn.steps.length) continue;
    lines.push("## Assistant", "");
    if (thinking.length) lines.push("<details><summary>Thinking</summary>", "", ...thinking, "", "</details>", "");
    if (tools.length) lines.push("Tools:", "", ...tools.map(toolLine), "");
    if (answer) lines.push(answer, "");
    const stats = turnStats(turn, Date.now());
    lines.push(
      `*${[stats.modelID, formatDuration(stats.elapsedMs), `${formatTokens(stats.tokens.input)} in`, `${formatTokens(stats.tokens.output)} out`]
        .filter(Boolean)
        .join(" · ")}*`,
      "",
    );
  }
  return `${lines.join("\n").trimEnd()}\n`;
}

/** A file name for the export: the chat title, made safe. */
export function exportFileName(session: Session | undefined): string {
  const base = (session?.title || "chat").replace(/[^\p{L}\p{N}\- ]+/gu, "").trim().replace(/\s+/g, "-").slice(0, 60);
  return `${base || "chat"}.md`;
}
