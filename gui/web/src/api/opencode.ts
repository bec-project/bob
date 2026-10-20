import type {
  Agent,
  BobConnection,
  McpConfigEntry,
  McpStatus,
  MessageEntry,
  ModelRef,
  PermissionRequest,
  Provider,
  QuestionRequest,
  Session,
  SessionStatus,
  SnapshotFileDiff,
  Todo,
} from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

export interface PromptInput {
  text: string;
  model?: ModelRef;
  agent?: string;
  variant?: string;
}

/** Answers to an OpenCode question request: one list of chosen labels per question. */
export type QuestionAnswers = string[][];

function basicAuth(username: string, password: string): string {
  const bytes = new TextEncoder().encode(`${username}:${password}`);
  return `Basic ${btoa(String.fromCharCode(...bytes))}`;
}

/** Normalises the base URL bob prints (http://127.0.0.1:PORT, maybe with a trailing slash). */
export function baseUrl(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/**
 * A small client for the OpenCode server API that bob's container serves.
 * Every request carries HTTP Basic auth with bob's generated password; OpenCode's CORS policy
 * admits localhost pages and Tauri webviews.
 */
export class OpencodeApi {
  readonly base: string;
  readonly auth: string;

  constructor(connection: BobConnection) {
    this.base = baseUrl(connection.url);
    this.auth = basicAuth(connection.username ?? "opencode", connection.password);
  }

  async request<T>(
    method: string,
    path: string,
    options: { body?: unknown; query?: Record<string, string | number | boolean | undefined>; signal?: AbortSignal } = {},
  ): Promise<T> {
    const url = new URL(this.base + path);
    for (const [key, value] of Object.entries(options.query ?? {})) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
    const headers: Record<string, string> = { Authorization: this.auth, Accept: "application/json" };
    if (options.body !== undefined) headers["Content-Type"] = "application/json";
    let response: Response;
    try {
      response = await fetch(url, {
        method,
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: options.signal,
      });
    } catch (error) {
      if ((error as Error).name === "AbortError") throw error;
      throw new ApiError(0, `Cannot reach ${this.base}: ${(error as Error).message}`);
    }
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      const message =
        response.status === 401
          ? "The server refused the password. Paste the connection bob printed for this launch."
          : `${method} ${path} failed (${response.status})${text ? `: ${text.slice(0, 300)}` : ""}`;
      throw new ApiError(response.status, message);
    }
    if (response.status === 204) return undefined as T;
    const type = response.headers.get("content-type") ?? "";
    if (!type.includes("json")) return (await response.text()) as T;
    return (await response.json()) as T;
  }

  health() {
    return this.request<{ healthy: boolean; version: string }>("GET", "/global/health");
  }

  sessions() {
    return this.request<Session[]>("GET", "/session", { query: { roots: true } });
  }

  createSession(title?: string) {
    return this.request<Session>("POST", "/session", { body: title ? { title } : {} });
  }

  renameSession(id: string, title: string) {
    return this.request<Session>("PATCH", `/session/${id}`, { body: { title } });
  }

  deleteSession(id: string) {
    return this.request<boolean>("DELETE", `/session/${id}`);
  }

  sessionStatus() {
    return this.request<Record<string, SessionStatus>>("GET", "/session/status");
  }

  messages(sessionID: string) {
    return this.request<MessageEntry[]>("GET", `/session/${sessionID}/message`);
  }

  prompt(sessionID: string, input: PromptInput) {
    return this.request<void>("POST", `/session/${sessionID}/prompt_async`, {
      body: {
        parts: [{ type: "text", text: input.text }],
        model: input.model,
        agent: input.agent,
        variant: input.variant,
      },
    });
  }

  /** Summarises the chat so far and continues from the summary (OpenCode's compaction). */
  compact(sessionID: string, model: ModelRef) {
    return this.request<boolean>("POST", `/session/${sessionID}/summarize`, {
      body: { providerID: model.providerID, modelID: model.modelID },
    });
  }

  /** Files the chat changed in the workspace, or those one message changed. */
  diff(sessionID: string, messageID?: string) {
    return this.request<SnapshotFileDiff[]>("GET", `/session/${sessionID}/diff`, { query: { messageID } });
  }

  /** Undoes a message and everything after it, files included, until the next prompt. */
  revert(sessionID: string, messageID: string) {
    return this.request<Session>("POST", `/session/${sessionID}/revert`, { body: { messageID } });
  }

  unrevert(sessionID: string) {
    return this.request<Session>("POST", `/session/${sessionID}/unrevert`);
  }

  /** A new chat with this chat's messages before messageID (all of them without one). */
  fork(sessionID: string, messageID?: string) {
    return this.request<Session>("POST", `/session/${sessionID}/fork`, { body: messageID ? { messageID } : {} });
  }

  todos(sessionID: string) {
    return this.request<Todo[]>("GET", `/session/${sessionID}/todo`);
  }

  abort(sessionID: string) {
    return this.request<boolean>("POST", `/session/${sessionID}/abort`);
  }

  providers() {
    return this.request<{ providers: Provider[]; default: Record<string, string> }>(
      "GET",
      "/config/providers",
    );
  }

  config() {
    return this.request<{
      model?: string;
      default_agent?: string;
      mcp?: Record<string, McpConfigEntry>;
    }>("GET", "/config");
  }

  agents() {
    return this.request<Agent[]>("GET", "/agent");
  }

  mcpStatus() {
    return this.request<Record<string, McpStatus>>("GET", "/mcp");
  }

  permissions() {
    return this.request<PermissionRequest[]>("GET", "/permission");
  }

  replyPermission(requestID: string, reply: "once" | "always" | "reject", message?: string) {
    return this.request<boolean>("POST", `/permission/${requestID}/reply`, {
      body: message ? { reply, message } : { reply },
    });
  }

  questions() {
    return this.request<QuestionRequest[]>("GET", "/question");
  }

  replyQuestion(requestID: string, answers: QuestionAnswers) {
    return this.request<boolean>("POST", `/question/${requestID}/reply`, { body: { answers } });
  }

  rejectQuestion(requestID: string) {
    return this.request<boolean>("POST", `/question/${requestID}/reject`);
  }
}
