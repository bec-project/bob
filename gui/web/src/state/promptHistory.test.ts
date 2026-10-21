import { describe, expect, it } from "vitest";
import type { MessageEntry } from "../api/types";
import { promptHistory, recallStep } from "./promptHistory";

const message = (role: "user" | "assistant", text: string, synthetic = false) =>
  ({ info: { role }, parts: [{ type: "text", text, synthetic }] }) as unknown as MessageEntry;

describe("promptHistory", () => {
  it("takes the chat's own prompts, oldest first, without repeats in a row", () => {
    const entries = [
      message("user", "list devices"),
      message("assistant", "samx, samy"),
      message("user", "read samx"),
      message("user", "read samx"),
      message("user", "added by OpenCode", true),
    ];
    expect(promptHistory(entries, ["elsewhere"])).toEqual(["list devices", "read samx"]);
  });

  it("falls back to the prompts sent from this window in a new chat", () => {
    expect(promptHistory(undefined, ["a", "b"])).toEqual(["a", "b"]);
    expect(promptHistory([message("assistant", "hi")], [])).toEqual([]);
  });
});

describe("recallStep", () => {
  const history = ["first", "second", "third"];

  it("steps back from the newest prompt and stops at the oldest", () => {
    let step = recallStep(history, undefined, "my draft", "up");
    expect(step).toEqual({ recall: { index: 2, draft: "my draft" }, text: "third" });
    step = recallStep(history, step!.recall, step!.text, "up");
    step = recallStep(history, step!.recall, step!.text, "up");
    expect(step?.text).toBe("first");
    expect(recallStep(history, step!.recall, step!.text, "up")).toBeUndefined();
  });

  it("steps forward and gives the draft back after the newest", () => {
    const down = recallStep(history, { index: 1, draft: "my draft" }, "second", "down");
    expect(down).toEqual({ recall: { index: 2, draft: "my draft" }, text: "third" });
    expect(recallStep(history, down!.recall, "third", "down")).toEqual({ recall: undefined, text: "my draft" });
  });

  it("leaves the keys alone without history or outside it", () => {
    expect(recallStep([], undefined, "", "up")).toBeUndefined();
    expect(recallStep(history, undefined, "", "down")).toBeUndefined();
  });
});
