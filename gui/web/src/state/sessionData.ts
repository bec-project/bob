import type {
  Event,
  MessageEntry,
  Part,
  PermissionRequest,
  QuestionRequest,
  Session,
  SessionStatus,
  SnapshotFileDiff,
  Todo,
} from "../api/types";

/** The workspace changes one prompt led to (OpenCode diffs the snapshots around it). */
export interface PromptChanges {
  messageID: string;
  prompt: string;
  files: SnapshotFileDiff[];
}

/** Everything the event stream keeps current. Updates are immutable so React can compare. */
export interface SessionData {
  sessions: Record<string, Session>;
  status: Record<string, SessionStatus>;
  /** Messages of the sessions that were opened, oldest first. */
  messages: Record<string, MessageEntry[]>;
  permissions: Record<string, PermissionRequest>;
  questions: Record<string, QuestionRequest>;
  /** The last error a session reported, until its next prompt. */
  errors: Record<string, string>;
  /** What each prompt of a chat changed in the workspace, oldest first. */
  changes: Record<string, PromptChanges[]>;
  /** The agent's todo list per chat. */
  todos: Record<string, Todo[]>;
}

export const emptySessionData: SessionData = {
  sessions: {},
  status: {},
  messages: {},
  permissions: {},
  questions: {},
  errors: {},
  changes: {},
  todos: {},
};

// OpenCode ids grow with time within a session, so sorting by id keeps the server's order.
const byId = (a: { id: string }, b: { id: string }) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);

function without<T>(record: Record<string, T>, key: string): Record<string, T> {
  if (!(key in record)) return record;
  const next = { ...record };
  delete next[key];
  return next;
}

function upsertMessage(list: MessageEntry[], entry: MessageEntry): MessageEntry[] {
  const index = list.findIndex((item) => item.info.id === entry.info.id);
  if (index === -1) return [...list, entry].sort((a, b) => byId(a.info, b.info));
  const next = list.slice();
  next[index] = entry;
  return next;
}

function updateParts(
  data: SessionData,
  sessionID: string,
  messageID: string,
  change: (parts: Part[]) => Part[],
): SessionData {
  const list = data.messages[sessionID];
  if (!list) return data;
  const index = list.findIndex((item) => item.info.id === messageID);
  if (index === -1) return data;
  const entry = list[index];
  const parts = change(entry.parts);
  if (parts === entry.parts) return data;
  const next = list.slice();
  next[index] = { ...entry, parts };
  return { ...data, messages: { ...data.messages, [sessionID]: next } };
}

export function errorText(error: unknown): string {
  if (!error || typeof error !== "object") return String(error ?? "Unknown error");
  const value = error as { name?: string; message?: string; data?: { message?: string } };
  const message = value.data?.message ?? value.message;
  if (value.name === "MessageAbortedError") return "Stopped.";
  return message ? message : (value.name ?? "Unknown error");
}

/** Applies one server event. Returns the same object when the event changes nothing here. */
export function applyEvent(data: SessionData, event: Event): SessionData {
  switch (event.type) {
    case "session.created":
    case "session.updated": {
      const info = event.properties.info;
      if (info.parentID) return data;
      return { ...data, sessions: { ...data.sessions, [info.id]: info } };
    }
    case "session.deleted": {
      const id = event.properties.info.id;
      return {
        ...data,
        sessions: without(data.sessions, id),
        messages: without(data.messages, id),
        status: without(data.status, id),
        changes: without(data.changes, id),
        todos: without(data.todos, id),
      };
    }
    case "session.status": {
      const { sessionID, status } = event.properties;
      return { ...data, status: { ...data.status, [sessionID]: status } };
    }
    case "session.idle": {
      const { sessionID } = event.properties;
      return { ...data, status: { ...data.status, [sessionID]: { type: "idle" } } };
    }
    case "session.error": {
      const { sessionID, error } = event.properties;
      if (!sessionID || !error) return data;
      return { ...data, errors: { ...data.errors, [sessionID]: errorText(error) } };
    }
    case "message.updated": {
      const info = event.properties.info;
      const list = data.messages[info.sessionID];
      if (!list) return data;
      const existing = list.find((item) => item.info.id === info.id);
      const messages = upsertMessage(list, { info, parts: existing?.parts ?? [] });
      return { ...data, messages: { ...data.messages, [info.sessionID]: messages } };
    }
    case "message.removed": {
      const { sessionID, messageID } = event.properties;
      const list = data.messages[sessionID];
      if (!list) return data;
      return {
        ...data,
        messages: { ...data.messages, [sessionID]: list.filter((item) => item.info.id !== messageID) },
      };
    }
    case "message.part.updated": {
      const part = event.properties.part;
      const list = data.messages[part.sessionID];
      if (list && !list.some((item) => item.info.id === part.messageID)) {
        // The part arrived before its message: keep it on a placeholder until message.updated.
        return data;
      }
      return updateParts(data, part.sessionID, part.messageID, (parts) => {
        const index = parts.findIndex((item) => item.id === part.id);
        if (index === -1) return [...parts, part].sort(byId);
        const next = parts.slice();
        next[index] = part;
        return next;
      });
    }
    case "message.part.delta": {
      const { sessionID, messageID, partID, field, delta } = event.properties;
      return updateParts(data, sessionID, messageID, (parts) => {
        const index = parts.findIndex((item) => item.id === partID);
        if (index === -1) return parts;
        const part = parts[index] as Part & Record<string, unknown>;
        const current = part[field];
        if (typeof current !== "string" && current !== undefined) return parts;
        const next = parts.slice();
        next[index] = { ...part, [field]: (current ?? "") + delta } as Part;
        return next;
      });
    }
    case "message.part.removed": {
      const { sessionID, messageID, partID } = event.properties;
      return updateParts(data, sessionID, messageID, (parts) =>
        parts.some((item) => item.id === partID) ? parts.filter((item) => item.id !== partID) : parts,
      );
    }
    case "permission.asked": {
      const request = event.properties;
      return { ...data, permissions: { ...data.permissions, [request.id]: request } };
    }
    case "permission.replied":
      return { ...data, permissions: without(data.permissions, event.properties.requestID) };
    case "question.asked": {
      const request = event.properties;
      return { ...data, questions: { ...data.questions, [request.id]: request } };
    }
    case "question.replied":
    case "question.rejected":
      return { ...data, questions: without(data.questions, event.properties.requestID) };
    case "todo.updated": {
      const { sessionID, todos } = event.properties;
      return { ...data, todos: { ...data.todos, [sessionID]: todos } };
    }
    default:
      return data;
  }
}

/** Sessions newest first. */
export function sortedSessions(sessions: Record<string, Session>): Session[] {
  return Object.values(sessions).sort((a, b) => b.time.updated - a.time.updated);
}
