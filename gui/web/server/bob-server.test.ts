import { describe, expect, it } from "vitest";
import { fromOwnPage, loopbackTarget } from "./bob-server";

describe("loopbackTarget", () => {
  it("accepts http(s) on loopback names only", () => {
    expect(loopbackTarget("http://127.0.0.1:8000/mcp")?.port).toBe("8000");
    expect(loopbackTarget("http://localhost:8000/mcp")).not.toBeNull();
    expect(loopbackTarget("http://[::1]:8000/mcp")).not.toBeNull();
    expect(loopbackTarget("http://example.com/mcp")).toBeNull();
    expect(loopbackTarget("http://127.0.0.1.nip.io/mcp")).toBeNull();
    expect(loopbackTarget("http://user:pw@127.0.0.1/mcp")).toBeNull();
    expect(loopbackTarget("file:///etc/passwd")).toBeNull();
    expect(loopbackTarget(undefined)).toBeNull();
  });
});

describe("fromOwnPage", () => {
  it("needs the client header and a loopback Host", () => {
    expect(fromOwnPage({ "x-bob-gui": "1", host: "127.0.0.1:5173", "sec-fetch-site": "same-origin" })).toBe(true);
    expect(fromOwnPage({ "x-bob-gui": "1", host: "localhost:5173" })).toBe(true);
    expect(fromOwnPage({ host: "127.0.0.1:5173" })).toBe(false);
    expect(fromOwnPage({ "x-bob-gui": "1", host: "evil.example:5173" })).toBe(false);
    expect(fromOwnPage({ "x-bob-gui": "1", host: "127.0.0.1:5173", "sec-fetch-site": "cross-site" })).toBe(false);
  });
});
