import { describe, expect, it } from "vitest";
import { parseConnection, toConnection } from "./connection";
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

describe("toConnection", () => {
  it("takes the broker's connection object", () => {
    expect(toConnection({ url: "http://127.0.0.1:1", password: "p", container: "c" })).toEqual({
      url: "http://127.0.0.1:1",
      password: "p",
      container: "c",
    });
    expect(toConnection({ url: "http://127.0.0.1:1" })).toBeNull();
    expect(toConnection(null)).toBeNull();
    expect(toConnection("text")).toBeNull();
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
