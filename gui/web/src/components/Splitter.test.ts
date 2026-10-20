import { describe, expect, it } from "vitest";
import { clampWidth } from "./Splitter";

describe("clampWidth", () => {
  it("keeps a dragged width between the minimum and what leaves room for the chat", () => {
    expect(clampWidth(500.4, 280, 1600)).toBe(500);
    expect(clampWidth(100, 280, 1600)).toBe(280);
    expect(clampWidth(1500, 280, 1600)).toBe(1120);
    // a window too narrow for both still keeps the minimum
    expect(clampWidth(400, 280, 600)).toBe(280);
  });
});
