import { describe, expect, it } from "vitest";
import type { MessageEntry, Session } from "../api/types";
import { chatMarkdown, exportFileName } from "./exportMarkdown";

const session = { id: "ses_1", title: "Beamline health/check", time: { created: 0, updated: 0 } } as unknown as Session;
const entries = [
  { info: { id: "msg_1", role: "user", time: { created: 1000 }, sessionID: "ses_1", model: { providerID: "p", modelID: "m" } }, parts: [{ id: "p1", type: "text", text: "Is it healthy?" }] },
  {
    info: { id: "msg_2", role: "assistant", parentID: "msg_1", modelID: "Qwen", providerID: "p", time: { created: 1100, completed: 3100 }, tokens: { input: 1200, output: 40, reasoning: 5, cache: { read: 0, write: 0 } }, cost: 0 },
    parts: [
      { id: "p2", type: "reasoning", text: "Let me check.", time: { start: 1 } },
      { id: "p3", type: "tool", tool: "bec_mcp_bec_status", callID: "c", state: { status: "completed", input: {}, output: "{}", title: "", metadata: {}, time: { start: 1, end: 2 } } },
      { id: "p4", type: "text", text: "Yes, all services run." },
    ],
  },
] as unknown as MessageEntry[];

describe("chatMarkdown", () => {
  it("writes prompts, tools, answers and usage", () => {
    const md = chatMarkdown(session, entries);
    expect(md).toContain("# Beamline health/check");
    expect(md).toContain("## You\n\nIs it healthy?");
    expect(md).toContain("- `bec_mcp_bec_status`");
    expect(md).toContain("Yes, all services run.");
    expect(md).toContain("*Qwen · 2.1 s · 1.2k in · 40 out*");
    expect(md).not.toContain("Let me check.");
    expect(chatMarkdown(session, entries, { thinking: true })).toContain("Let me check.");
  });

  it("makes a safe file name", () => {
    expect(exportFileName(session)).toBe("Beamline-healthcheck.md");
    expect(exportFileName(undefined)).toBe("chat.md");
  });
});
