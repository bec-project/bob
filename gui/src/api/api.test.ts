import { describe, expect, it } from "vitest";
import { findResponse, hostUrl } from "../plugins/bec/mcpClient";
import { parseConnection } from "./connection";
import { SseParser } from "./events";

describe("parseConnection", () => {
  it("reads the JSON line bob prints", () => {
    const line = '{"url":"http://127.0.0.1:52997","password":"s3cret","container":"bob-1","readOnly":false,"runtime":"docker","workspace":"/w"}';
    expect(parseConnection(line)).toMatchObject({ url: "http://127.0.0.1:52997", password: "s3cret", container: "bob-1" });
  });

  it("takes a bare URL with a separate password", () => {
    expect(parseConnection("http://host:4096/", "pw")).toEqual({ url: "http://host:4096", password: "pw" });
  });

  it("rejects anything else", () => {
    expect(parseConnection("")).toBeNull();
    expect(parseConnection('{"url":"http://x"}')).toBeNull();
    expect(parseConnection("ftp://x")).toBeNull();
    expect(parseConnection("{broken")).toBeNull();
  });
});

describe("SseParser", () => {
  it("joins data lines and splits events across chunks", () => {
    const parser = new SseParser();
    expect(parser.push('data: {"a":')).toEqual([]);
    expect(parser.push('1}\n\ndata: {"b":2}\r\n')).toEqual(['{"a":1}']);
    expect(parser.push("\r\n: comment\n\n")).toEqual(['{"b":2}']);
  });
});

describe("MCP helpers", () => {
  it("finds the answer in an event-stream body", () => {
    const body = 'event: message\ndata: {"jsonrpc":"2.0","id":3,"result":{"ok":true}}\n\n';
    expect(findResponse(body, "text/event-stream", 3)?.result).toEqual({ ok: true });
    expect(findResponse('{"jsonrpc":"2.0","id":4,"result":1}', "application/json", 4)?.result).toBe(1);
    expect(findResponse(body, "text/event-stream", 9)).toBeUndefined();
  });

  it("maps the container's name for the host to loopback", () => {
    expect(hostUrl("http://host.docker.internal:8000/mcp")).toBe("http://127.0.0.1:8000/mcp");
    expect(hostUrl("http://host.containers.internal:19000/mcp")).toBe("http://127.0.0.1:19000/mcp");
    expect(hostUrl("https://mcp.example/mcp")).toBe("https://mcp.example/mcp");
  });
});
