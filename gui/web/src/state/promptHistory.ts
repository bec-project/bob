import type { MessageEntry } from "../api/types";

/** The text the user typed in a message, without parts OpenCode added. */
function promptText(entry: MessageEntry): string {
  return entry.parts
    .filter((part) => part.type === "text" && !part.synthetic)
    .map((part) => (part.type === "text" ? part.text : ""))
    .join("\n")
    .trim();
}

/**
 * The prompts the Up arrow steps through, oldest first: the open chat's own, or, in a chat that has
 * none yet, the ones sent from this window. Repeats in a row count once.
 */
export function promptHistory(entries: MessageEntry[] | undefined, sent: string[]): string[] {
  const own = (entries ?? [])
    .filter((entry) => entry.info.role === "user")
    .map(promptText)
    .filter(Boolean);
  const list = own.length ? own : sent;
  return list.filter((text, i) => text !== list[i - 1]);
}

/** Where the composer stands in the history while the user steps through it. */
export interface Recall {
  index: number;
  /** What the composer held before the first step, given back after the newest prompt. */
  draft: string;
}

/**
 * One press of Up or Down: the next position and the text to show, or undefined when the key
 * should move the caret as usual (no older prompt, or not stepping through the history).
 */
export function recallStep(
  history: string[],
  recall: Recall | undefined,
  current: string,
  direction: "up" | "down",
): { recall: Recall | undefined; text: string } | undefined {
  if (direction === "up") {
    if (!recall) {
      return history.length ? { recall: { index: history.length - 1, draft: current }, text: history[history.length - 1] } : undefined;
    }
    if (recall.index === 0) return undefined;
    return { recall: { ...recall, index: recall.index - 1 }, text: history[recall.index - 1] };
  }
  if (!recall) return undefined;
  if (recall.index >= history.length - 1) return { recall: undefined, text: recall.draft };
  return { recall: { ...recall, index: recall.index + 1 }, text: history[recall.index + 1] };
}
