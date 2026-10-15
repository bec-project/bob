import { describe, expect, it } from "vitest";
import type { Event, MessageEntry } from "../api/types";
import { applyEvent, emptySessionData, type SessionData } from "./sessionData";
import { groupTurns, turnStats } from "./turns";

const ev = (type: string, properties: unknown) => ({ id: "evt", type, properties }) as unknown as Event;

const user = (id: string, created: number): MessageEntry =>
  ({
    info: { id, sessionID: "ses_1", role: "user", time: { created }, agent: "build", model: { providerID: "p", modelID: "m" } },
    parts: [{ id: `prt_${id}`, sessionID: "ses_1", messageID: id, type: "text", text: "hi" }],
  }) as MessageEntry;

const assistant = (id: string, parentID: string, created: number, completed?: number, tokens = 10): MessageEntry =>
  ({
    info: {
      id,
      sessionID: "ses_1",
      role: "assistant",
      parentID,
      time: completed ? { created, completed } : { created },
      modelID: "m",
      providerID: "p",
      mode: "build",
      agent: "build",
      path: { cwd: "/", root: "/" },
      cost: 0,
      tokens: { input: tokens, output: 2, reasoning: 1, cache: { read: 0, write: 0 } },
    },
    parts: [],
  }) as unknown as MessageEntry;

function withSession(messages: MessageEntry[]): SessionData {
  return { ...emptySessionData, messages: { ses_1: messages } };
}

describe("applyEvent", () => {
  it("streams text deltas into the right part", () => {
    let data = withSession([user("msg_1", 1), assistant("msg_2", "msg_1", 2)]);
    data = applyEvent(
      data,
      ev("message.part.updated", {
        part: { id: "prt_a", sessionID: "ses_1", messageID: "msg_2", type: "text", text: "" },
      }),
    );
    data = applyEvent(data, ev("message.part.delta", { sessionID: "ses_1", messageID: "msg_2", partID: "prt_a", field: "text", delta: "Hel" }));
    data = applyEvent(data, ev("message.part.delta", { sessionID: "ses_1", messageID: "msg_2", partID: "prt_a", field: "text", delta: "lo" }));
    const part = data.messages.ses_1[1].parts[0];
    expect(part.type === "text" && part.text).toBe("Hello");
  });

  it("returns the same object for events it does not use", () => {
    const data = withSession([]);
    expect(applyEvent(data, ev("server.heartbeat", {}))).toBe(data);
    expect(applyEvent(data, ev("message.part.delta", { sessionID: "ses_x", messageID: "m", partID: "p", field: "text", delta: "x" }))).toBe(data);
  });

  it("keeps messages in id order and replaces updated ones", () => {
    let data = withSession([user("msg_1", 1)]);
    data = applyEvent(data, ev("message.updated", { info: assistant("msg_3", "msg_1", 3).info }));
    data = applyEvent(data, ev("message.updated", { info: assistant("msg_2", "msg_1", 2).info }));
    data = applyEvent(data, ev("message.updated", { info: assistant("msg_2", "msg_1", 2, 5).info }));
    expect(data.messages.ses_1.map((m) => m.info.id)).toEqual(["msg_1", "msg_2", "msg_3"]);
    expect((data.messages.ses_1[1].info as { time: { completed?: number } }).time.completed).toBe(5);
  });

  it("tracks permission requests until they are answered", () => {
    let data = withSession([]);
    data = applyEvent(data, ev("permission.asked", { id: "per_1", sessionID: "ses_1", permission: "bash", patterns: ["ls"], metadata: {}, always: [] }));
    expect(Object.keys(data.permissions)).toEqual(["per_1"]);
    data = applyEvent(data, ev("permission.replied", { sessionID: "ses_1", requestID: "per_1", reply: "once" }));
    expect(data.permissions).toEqual({});
  });

  it("drops a deleted session and its messages", () => {
    let data: SessionData = { ...withSession([user("msg_1", 1)]), sessions: { ses_1: { id: "ses_1" } as never } };
    data = applyEvent(data, ev("session.deleted", { sessionID: "ses_1", info: { id: "ses_1" } }));
    expect(data.sessions).toEqual({});
    expect(data.messages).toEqual({});
  });

  it("records session errors in words", () => {
    const data = applyEvent(withSession([]), ev("session.error", { sessionID: "ses_1", error: { name: "APIError", data: { message: "rate limited" } } }));
    expect(data.errors.ses_1).toBe("rate limited");
  });
});

describe("turns", () => {
  it("groups the steps of each prompt and sums their usage", () => {
    const entries = [
      user("msg_1", 1000),
      assistant("msg_2", "msg_1", 1100, 2000, 100),
      assistant("msg_3", "msg_1", 2000, 4000, 150),
      user("msg_4", 5000),
      assistant("msg_5", "msg_4", 5100),
    ];
    const turns = groupTurns(entries);
    expect(turns).toHaveLength(2);
    expect(turns[0].steps).toHaveLength(2);
    const done = turnStats(turns[0], 99_999);
    expect(done).toMatchObject({ elapsedMs: 3000, running: false, contextTokens: 150 });
    expect(done.tokens.input).toBe(250);
    const running = turnStats(turns[1], 6000);
    expect(running).toMatchObject({ running: true, elapsedMs: 1000 });
  });
});

describe("todos", () => {
  it("keeps the latest todo list per session and drops it with the session", () => {
    let data = withSession([]);
    data = applyEvent(data, ev("todo.updated", { sessionID: "ses_1", todos: [{ content: "x", status: "pending", priority: "high" }] }));
    expect(data.todos.ses_1).toHaveLength(1);
    data = applyEvent(data, ev("session.deleted", { sessionID: "ses_1", info: { id: "ses_1" } }));
    expect(data.todos).toEqual({});
  });
});
