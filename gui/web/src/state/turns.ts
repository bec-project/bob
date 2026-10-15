import type { AssistantMessage, MessageEntry, UserMessage } from "../api/types";

export interface Tokens {
  input: number;
  output: number;
  reasoning: number;
  cacheRead: number;
  cacheWrite: number;
}

export const noTokens: Tokens = { input: 0, output: 0, reasoning: 0, cacheRead: 0, cacheWrite: 0 };

export function addTokens(a: Tokens, b: Tokens): Tokens {
  return {
    input: a.input + b.input,
    output: a.output + b.output,
    reasoning: a.reasoning + b.reasoning,
    cacheRead: a.cacheRead + b.cacheRead,
    cacheWrite: a.cacheWrite + b.cacheWrite,
  };
}

export function messageTokens(info: AssistantMessage): Tokens {
  const t = info.tokens;
  return {
    input: t?.input ?? 0,
    output: t?.output ?? 0,
    reasoning: t?.reasoning ?? 0,
    cacheRead: t?.cache?.read ?? 0,
    cacheWrite: t?.cache?.write ?? 0,
  };
}

/** One prompt and every assistant message (model step) it produced. */
export interface Turn {
  id: string;
  user?: MessageEntry & { info: UserMessage };
  steps: (MessageEntry & { info: AssistantMessage })[];
}

export interface TurnStats {
  /** Milliseconds from the prompt to the last completed step, or until now while running. */
  elapsedMs: number;
  running: boolean;
  tokens: Tokens;
  cost: number;
  /** The context the last step sent: its input plus cached input. */
  contextTokens: number;
  modelID?: string;
  providerID?: string;
  variant?: string;
  toolCalls: number;
}

const isUser = (entry: MessageEntry): entry is MessageEntry & { info: UserMessage } =>
  entry.info.role === "user";
const isAssistant = (entry: MessageEntry): entry is MessageEntry & { info: AssistantMessage } =>
  entry.info.role === "assistant";

/** Groups messages into turns: each user message with the assistant messages answering it. */
export function groupTurns(entries: MessageEntry[]): Turn[] {
  const turns: Turn[] = [];
  const byUser = new Map<string, Turn>();
  for (const entry of entries) {
    if (isUser(entry)) {
      const turn: Turn = { id: entry.info.id, user: entry, steps: [] };
      turns.push(turn);
      byUser.set(entry.info.id, turn);
    } else if (isAssistant(entry)) {
      const owner = byUser.get(entry.info.parentID) ?? turns[turns.length - 1];
      if (owner) owner.steps.push(entry);
      else turns.push({ id: entry.info.id, steps: [entry] });
    }
  }
  return turns;
}

export function turnStats(turn: Turn, now: number): TurnStats {
  let tokens = noTokens;
  let cost = 0;
  let toolCalls = 0;
  let running = false;
  let end = 0;
  const last = turn.steps[turn.steps.length - 1];
  for (const step of turn.steps) {
    tokens = addTokens(tokens, messageTokens(step.info));
    cost += step.info.cost ?? 0;
    toolCalls += step.parts.filter((part) => part.type === "tool").length;
    if (step.info.time.completed) end = Math.max(end, step.info.time.completed);
    else if (!step.info.error) running = true;
  }
  const start = turn.user?.info.time.created ?? turn.steps[0]?.info.time.created ?? now;
  // A step reports its tokens when it finishes, so the context is the last step that has some.
  const measured = [...turn.steps].reverse().find((step) => (step.info.tokens?.input ?? 0) > 0);
  const lastTokens = measured ? messageTokens(measured.info) : noTokens;
  return {
    // a stopped step may have no completion time: end the turn where its last step began
    elapsedMs: Math.max(0, (running ? now : end || (last?.info.time.created ?? start)) - start),
    running,
    tokens,
    cost,
    contextTokens: lastTokens.input + lastTokens.cacheRead,
    modelID: last?.info.modelID ?? turn.user?.info.model.modelID,
    providerID: last?.info.providerID ?? turn.user?.info.model.providerID,
    variant: last?.info.variant ?? turn.user?.info.model.variant,
    toolCalls,
  };
}

export function formatTokens(value: number): string {
  if (value < 1000) return String(value);
  if (value < 10_000) return `${(value / 1000).toFixed(1)}k`;
  if (value < 1_000_000) return `${Math.round(value / 1000)}k`;
  return `${(value / 1_000_000).toFixed(1)}M`;
}

export function formatDuration(ms: number): string {
  const seconds = ms / 1000;
  if (seconds < 1) return `${Math.round(ms)} ms`;
  if (seconds < 60) return `${seconds.toFixed(1)} s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ${Math.round(seconds % 60)} s`;
  return `${Math.floor(minutes / 60)} h ${minutes % 60} min`;
}

export function formatCost(cost: number): string {
  return cost >= 0.01 ? `$${cost.toFixed(2)}` : `$${cost.toFixed(4)}`;
}
