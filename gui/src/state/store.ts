import { create } from "zustand";
import { parseConnection, saveConnection } from "../api/connection";
import { startBob as startBobProcess } from "../platform";
import { followEvents, type StreamState } from "../api/events";
import { ApiError, OpencodeApi, type QuestionAnswers } from "../api/opencode";
import type {
  Agent,
  BobConnection,
  McpConfigEntry,
  McpStatus,
  Model,
  ModelRef,
  Provider,
} from "../api/types";
import { loadPrefs, savePrefs, type Prefs } from "./prefs";
import { applyEvent, emptySessionData, sortedSessions, type SessionData } from "./sessionData";
import { groupTurns } from "./turns";

export type Phase = "setup" | "connecting" | "starting" | "ready";

interface ComposerRequest {
  text: string;
  nonce: number;
  /** Replace the composer's text instead of adding to it. */
  replace?: boolean;
}

export interface State extends SessionData {
  phase: Phase;
  /** Why the last connection attempt failed, shown on the connect screen. */
  connectError?: string;
  connection?: BobConnection;
  api?: OpencodeApi;
  stream: StreamState;
  streamDetail?: string;
  serverVersion?: string;
  currentId?: string;
  loading: Record<string, boolean>;
  providers: Provider[];
  defaultModel?: ModelRef;
  agents: Agent[];
  defaultAgent: string;
  mcpConfig: Record<string, McpConfigEntry>;
  mcpStatus: Record<string, McpStatus>;
  prefs: Prefs;
  composer?: ComposerRequest;
  /** A transient message for the user (a failed request), cleared by the next action. */
  notice?: { text: string; tone: "error" | "info" };

  connect(connection: BobConnection): Promise<void>;
  /** Desktop app: starts bob with a bob.toml, then connects to it. */
  startBob(binary: string, config: string): Promise<void>;
  /** The bob.toml being started, while phase is "starting". */
  starting?: string;
  disconnect(): void;
  selectSession(id: string | undefined): void;
  send(text: string): Promise<void>;
  abort(): Promise<void>;
  renameSession(id: string, title: string): Promise<void>;
  deleteSession(id: string): Promise<void>;
  replyPermission(id: string, reply: "once" | "always" | "reject", message?: string): Promise<void>;
  replyQuestion(id: string, answers: QuestionAnswers): Promise<void>;
  rejectQuestion(id: string): Promise<void>;
  setPrefs(patch: Partial<Prefs>): void;
  insertPrompt(text: string): void;
  dismissNotice(): void;
  /** Summarises the open chat so the next prompts start from a short context. */
  compact(): Promise<void>;
  /** Undoes a user message and everything after it, workspace files included. */
  revert(messageID: string): Promise<void>;
  unrevert(): Promise<void>;
  /** Copies the open chat up to (not including) messageID into a new chat and opens it. */
  fork(messageID?: string): Promise<void>;
  /** Loads a sub-agent's chat so its messages show (and update) under its task call. */
  watchSession(id: string): void;
  setNotice(text: string): void;
}

let stopEvents: (() => void) | null = null;
let resyncTimers: Record<string, ReturnType<typeof setTimeout>> = {};

function parseModel(value: string | undefined): ModelRef | undefined {
  if (!value) return undefined;
  const slash = value.indexOf("/");
  if (slash <= 0) return undefined;
  return { providerID: value.slice(0, slash), modelID: value.slice(slash + 1) };
}

export function findModel(providers: Provider[], ref: ModelRef | undefined): Model | undefined {
  if (!ref) return undefined;
  return providers.find((p) => p.id === ref.providerID)?.models[ref.modelID];
}

/** The variant that switches thinking off, when the model's configuration defines one. */
export function noThinkingVariant(model: Model | undefined): string | undefined {
  return Object.keys(model?.variants ?? {}).find((name) => /no[-_ ]?think/i.test(name));
}

/** The model the next prompt uses: the user's pick when the server still offers it. */
export function activeModel(state: Pick<State, "prefs" | "providers" | "defaultModel">): ModelRef | undefined {
  if (state.prefs.model && findModel(state.providers, state.prefs.model)) return state.prefs.model;
  return state.defaultModel;
}

function promptText(entry: { parts: { type: string; text?: string; synthetic?: boolean }[] }): string {
  return entry.parts
    .filter((p) => p.type === "text" && !p.synthetic)
    .map((p) => p.text ?? "")
    .join("\n");
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export const useStore = create<State>((set, get) => {
  const fail = (error: unknown) => set({ notice: { text: message(error), tone: "error" } });

  const loadMessages = async (sessionID: string) => {
    const api = get().api;
    if (!api) return;
    set((s) => ({ loading: { ...s.loading, [sessionID]: true } }));
    try {
      const list = await api.messages(sessionID);
      set((s) => ({
        messages: { ...s.messages, [sessionID]: list },
        loading: { ...s.loading, [sessionID]: false },
      }));
      if (get().currentId === sessionID) void loadChanges(sessionID);
    } catch (error) {
      set((s) => ({ loading: { ...s.loading, [sessionID]: false } }));
      fail(error);
    }
  };

  // The session-wide diff stays empty in OpenCode 1.18, so changes are asked per prompt, and only
  // for the prompts whose answers report changed files (a patch part).
  const loadChanges = async (sessionID: string) => {
    const api = get().api;
    const entries = get().messages[sessionID];
    if (!api || !entries) return;
    const turns = groupTurns(entries).filter(
      (turn) => turn.user && turn.steps.some((step) => step.parts.some((part) => part.type === "patch")),
    );
    const changes = await Promise.all(
      turns.map(async (turn) => ({
        messageID: turn.user!.info.id,
        prompt: promptText(turn.user!),
        files: await api.diff(sessionID, turn.user!.info.id).catch(() => []),
      })),
    );
    set((s) => ({ changes: { ...s.changes, [sessionID]: changes.filter((c) => c.files.length > 0) } }));
  };

  const loadExtras = async (sessionID: string) => {
    const api = get().api;
    if (!api) return;
    const todos = await api.todos(sessionID).catch(() => undefined);
    set((s) => ({ todos: todos ? { ...s.todos, [sessionID]: todos } : s.todos }));
    await loadChanges(sessionID);
  };

  // After a turn ends, fetch the session once more so a missed event cannot leave it stale.
  const resync = (sessionID: string) => {
    clearTimeout(resyncTimers[sessionID]);
    resyncTimers[sessionID] = setTimeout(() => {
      if (get().messages[sessionID]) void loadMessages(sessionID);
      if (get().currentId === sessionID) void loadExtras(sessionID);
    }, 300);
  };

  const refreshLists = async (api: OpencodeApi) => {
    const [sessions, status, permissions, questions] = await Promise.all([
      api.sessions(),
      api.sessionStatus().catch(() => ({})),
      api.permissions().catch(() => []),
      api.questions().catch(() => []),
    ]);
    set({
      sessions: Object.fromEntries(sessions.map((s) => [s.id, s])),
      status,
      permissions: Object.fromEntries(permissions.map((p) => [p.id, p])),
      questions: Object.fromEntries(questions.map((q) => [q.id, q])),
    });
  };

  return {
    ...emptySessionData,
    phase: "setup",
    stream: "closed",
    loading: {},
    providers: [],
    agents: [],
    defaultAgent: "build",
    mcpConfig: {},
    mcpStatus: {},
    prefs: loadPrefs(),

    async connect(connection) {
      stopEvents?.();
      stopEvents = null;
      const api = new OpencodeApi(connection);
      set({ ...emptySessionData, phase: "connecting", connectError: undefined, api, connection });
      try {
        const health = await api.health().catch((error: unknown) => {
          // Older servers have no /global/health; the session list proves the password.
          if (error instanceof ApiError && error.status === 404) return undefined;
          throw error;
        });
        const [providers, config, agents, mcpStatus] = await Promise.all([
          api.providers(),
          api.config().catch(() => ({}) as Awaited<ReturnType<OpencodeApi["config"]>>),
          api.agents().catch(() => [] as Agent[]),
          api.mcpStatus().catch(() => ({})),
          refreshLists(api),
        ]);
        const fallback = Object.entries(providers.default)[0];
        const defaultModel =
          parseModel(config.model) ??
          (fallback ? { providerID: fallback[0], modelID: fallback[1] } : undefined);
        saveConnection(connection);
        const latest = sortedSessions(get().sessions)[0];
        set({
          phase: "ready",
          serverVersion: health?.version,
          providers: providers.providers,
          defaultModel,
          agents,
          defaultAgent: config.default_agent ?? "build",
          mcpConfig: config.mcp ?? {},
          mcpStatus,
          currentId: latest?.id,
        });
        if (latest) {
          void loadMessages(latest.id);
          void loadExtras(latest.id);
        }
        stopEvents = followEvents(api, {
          onEvent(event) {
            if (get().api !== api) return;
            const before = get();
            const after = applyEvent(before, event);
            if (after !== before) set(after);
            if (event.type === "session.idle") resync(event.properties.sessionID);
            if (event.type === "session.deleted" && get().currentId === event.properties.info.id) {
              set({ currentId: sortedSessions(get().sessions)[0]?.id });
            }
          },
          onOpen(reconnected) {
            if (!reconnected) return;
            void refreshLists(api).catch(fail);
            for (const id of Object.keys(get().messages)) void loadMessages(id);
            void api.mcpStatus().then((mcp) => set({ mcpStatus: mcp })).catch(() => undefined);
          },
          onState(stream, detail) {
            if (get().api === api) set({ stream, streamDetail: detail });
          },
          onUnauthorized() {
            if (get().api !== api) return;
            get().disconnect();
            set({ connectError: "The server no longer accepts this password. Paste the connection bob printed for its current launch." });
          },
        });
      } catch (error) {
        saveConnection(null);
        set({ phase: "setup", api: undefined, connectError: message(error) });
      }
    },

    async startBob(binary, config) {
      set({ phase: "starting", starting: config, connectError: undefined });
      try {
        const line = await startBobProcess(binary, config);
        const connection = parseConnection(line);
        if (!connection) throw new Error(`bob printed an unexpected connection line: ${line.slice(0, 200)}`);
        set({ starting: undefined });
        await get().connect(connection);
      } catch (error) {
        set({ phase: "setup", starting: undefined, connectError: message(error) });
      }
    },

    disconnect() {
      stopEvents?.();
      stopEvents = null;
      for (const timer of Object.values(resyncTimers)) clearTimeout(timer);
      resyncTimers = {};
      saveConnection(null);
      set({
        ...emptySessionData,
        phase: "setup",
        api: undefined,
        connection: undefined,
        currentId: undefined,
        stream: "closed",
        connectError: undefined,
      });
    },

    selectSession(id) {
      set({ currentId: id, notice: undefined });
      if (id && !get().messages[id]) void loadMessages(id);
      if (id) void loadExtras(id);
    },

    watchSession(id) {
      if (!get().messages[id] && !get().loading[id]) void loadMessages(id);
    },

    async compact() {
      const { api, currentId } = get();
      const model = activeModel(get());
      if (!api || !currentId || !model) return;
      await api.compact(currentId, model).catch(fail);
    },

    async revert(messageID) {
      const { api, currentId, messages } = get();
      if (!api || !currentId) return;
      try {
        const session = await api.revert(currentId, messageID);
        set((s) => ({ sessions: { ...s.sessions, [currentId]: session } }));
        void loadExtras(currentId);
        // like OpenCode's /undo: the undone prompt goes back into the composer
        const entry = messages[currentId]?.find((m) => m.info.id === messageID);
        const text = entry?.parts
          .filter((p) => p.type === "text" && !p.synthetic)
          .map((p) => (p.type === "text" ? p.text : ""))
          .join("\n");
        if (text) set({ composer: { text, nonce: Date.now(), replace: true } });
      } catch (error) {
        fail(error);
      }
    },

    async unrevert() {
      const { api, currentId } = get();
      if (!api || !currentId) return;
      try {
        const session = await api.unrevert(currentId);
        // the undone prompt went into the composer; redo takes it back out
        set((s) => ({ sessions: { ...s.sessions, [currentId]: session }, composer: { text: "", nonce: Date.now(), replace: true } }));
        void loadExtras(currentId);
      } catch (error) {
        fail(error);
      }
    },

    async fork(messageID) {
      const { api, currentId } = get();
      if (!api || !currentId) return;
      try {
        const session = await api.fork(currentId, messageID);
        set((s) => ({ sessions: { ...s.sessions, [session.id]: session } }));
        get().selectSession(session.id);
      } catch (error) {
        fail(error);
      }
    },

    setNotice(text) {
      set({ notice: { text, tone: "info" } });
    },

    async send(text) {
      const state = get();
      const api = state.api;
      if (!api || !text.trim()) return;
      let sessionID = state.currentId;
      try {
        if (!sessionID) {
          const session = await api.createSession();
          sessionID = session.id;
          set((s) => ({
            sessions: { ...s.sessions, [session.id]: session },
            messages: { ...s.messages, [session.id]: [] },
            currentId: session.id,
          }));
        }
        const model = activeModel(get());
        const variant = get().prefs.thinking
          ? undefined
          : noThinkingVariant(findModel(get().providers, model));
        const agent = get().prefs.agent ?? undefined;
        set((s) => {
          const errors = { ...s.errors };
          delete errors[sessionID!];
          return { errors, notice: undefined };
        });
        await api.prompt(sessionID, { text, model, agent, variant });
      } catch (error) {
        fail(error);
      }
    },

    async abort() {
      const { api, currentId } = get();
      if (!api || !currentId) return;
      await api.abort(currentId).catch(fail);
    },

    async renameSession(id, title) {
      const api = get().api;
      if (!api) return;
      try {
        const session = await api.renameSession(id, title);
        set((s) => ({ sessions: { ...s.sessions, [id]: session } }));
      } catch (error) {
        fail(error);
      }
    },

    async deleteSession(id) {
      const api = get().api;
      if (!api) return;
      try {
        await api.deleteSession(id);
        set((s) => {
          const sessions = { ...s.sessions };
          const messages = { ...s.messages };
          delete sessions[id];
          delete messages[id];
          return { sessions, messages };
        });
        if (get().currentId === id) get().selectSession(sortedSessions(get().sessions)[0]?.id);
      } catch (error) {
        fail(error);
      }
    },

    async replyPermission(id, reply, message) {
      const api = get().api;
      if (!api) return;
      try {
        await api.replyPermission(id, reply, message);
        set((s) => {
          const permissions = { ...s.permissions };
          delete permissions[id];
          return { permissions };
        });
      } catch (error) {
        fail(error);
      }
    },

    async replyQuestion(id, answers) {
      const api = get().api;
      if (!api) return;
      try {
        await api.replyQuestion(id, answers);
        set((s) => {
          const questions = { ...s.questions };
          delete questions[id];
          return { questions };
        });
      } catch (error) {
        fail(error);
      }
    },

    async rejectQuestion(id) {
      const api = get().api;
      if (!api) return;
      try {
        await api.rejectQuestion(id);
        set((s) => {
          const questions = { ...s.questions };
          delete questions[id];
          return { questions };
        });
      } catch (error) {
        fail(error);
      }
    },

    setPrefs(patch) {
      const prefs = { ...get().prefs, ...patch };
      savePrefs(prefs);
      set({ prefs });
    },

    insertPrompt(text) {
      set({ composer: { text, nonce: Date.now() } });
    },

    dismissNotice() {
      set({ notice: undefined });
    },
  };
});

/** Whether a plugin is switched on, falling back to its default. */
export function pluginEnabled(prefs: Prefs, id: string, defaultEnabled: boolean): boolean {
  return prefs.plugins[id] ?? defaultEnabled;
}
