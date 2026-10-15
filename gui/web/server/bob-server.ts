import { readFile } from "node:fs/promises";
import type { IncomingMessage, ServerResponse } from "node:http";
import { Readable } from "node:stream";
import type { Connect, Plugin } from "vite";

// Requests from the page carry this header. Browsers cannot add it to a cross-site request
// without a CORS preflight, which this middleware never approves.
export const CLIENT_HEADER = "x-bob-gui";
export const TARGET_HEADER = "x-bob-target";

const LOOPBACK_NAMES = new Set(["127.0.0.1", "localhost", "[::1]"]);
const FORWARD_REQUEST = ["accept", "authorization", "content-type", "last-event-id",
  "mcp-protocol-version", "mcp-session-id"];
const FORWARD_RESPONSE = ["cache-control", "content-type", "mcp-session-id", "www-authenticate"];

/** The target URL when it is an http(s) URL on this machine's loopback interface. */
export function loopbackTarget(raw: string | undefined): URL | null {
  if (!raw) return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return null;
  if (url.username || url.password) return null;
  return LOOPBACK_NAMES.has(url.hostname) ? url : null;
}

/** Whether the request came from this server's own page rather than another site. */
export function fromOwnPage(headers: IncomingMessage["headers"]): boolean {
  if (headers[CLIENT_HEADER] !== "1") return false;
  const host = String(headers.host ?? "").replace(/:\d+$/, "");
  if (!LOOPBACK_NAMES.has(host)) return false;
  const site = headers["sec-fetch-site"];
  return site === undefined || site === "same-origin" || site === "none";
}

function send(res: ServerResponse, status: number, body: string, type = "text/plain") {
  res.statusCode = status;
  res.setHeader("content-type", `${type}; charset=utf-8`);
  res.setHeader("cache-control", "no-store");
  res.end(body);
}

async function readBody(req: IncomingMessage): Promise<Buffer> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks);
}

async function connection(res: ServerResponse) {
  const file = process.env.BOB_CONNECTION_FILE;
  if (!file) {
    res.statusCode = 204;
    res.end();
    return;
  }
  try {
    const text = await readFile(file, "utf8");
    JSON.parse(text);
    send(res, 200, text, "application/json");
  } catch (error) {
    send(res, 503, `cannot read BOB_CONNECTION_FILE: ${(error as Error).message}`);
  }
}

async function relay(req: IncomingMessage, res: ServerResponse) {
  const method = req.method ?? "GET";
  if (!["GET", "POST", "DELETE"].includes(method)) {
    send(res, 405, "method not allowed");
    return;
  }
  const target = loopbackTarget(req.headers[TARGET_HEADER] as string | undefined);
  if (!target) {
    send(res, 400, `${TARGET_HEADER} must be an http(s) URL on 127.0.0.1, localhost or [::1]`);
    return;
  }
  const headers = new Headers();
  for (const name of FORWARD_REQUEST) {
    const value = req.headers[name];
    if (typeof value === "string") headers.set(name, value);
  }
  const abort = new AbortController();
  res.on("close", () => abort.abort());
  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method,
      headers,
      body: method === "POST" ? new Uint8Array(await readBody(req)) : undefined,
      redirect: "manual",
      signal: abort.signal,
    });
  } catch (error) {
    if (!abort.signal.aborted) send(res, 502, `cannot reach ${target.origin}: ${(error as Error).message}`);
    return;
  }
  res.statusCode = upstream.status;
  for (const name of FORWARD_RESPONSE) {
    const value = upstream.headers.get(name);
    if (value !== null) res.setHeader(name, value);
  }
  if (!upstream.body) {
    res.end();
    return;
  }
  Readable.fromWeb(upstream.body as never)
    .on("error", () => res.destroy())
    .pipe(res);
}

function middleware(): Connect.NextHandleFunction {
  return (req, res, next) => {
    const path = (req.url ?? "").split("?")[0];
    if (!path.startsWith("/__bob/")) return next();
    if (!fromOwnPage(req.headers)) {
      send(res, 403, "forbidden");
      return;
    }
    const handle =
      path === "/__bob/connection" ? connection(res)
      : path === "/__bob/mcp" ? relay(req, res)
      : Promise.resolve(send(res, 404, "not found"));
    handle.catch((error: Error) => {
      if (!res.headersSent) send(res, 500, error.message);
      else res.destroy();
    });
  };
}

/** Vite plugin that serves /__bob/* on the dev and preview servers. */
export function bobServer(): Plugin {
  return {
    name: "bob-server",
    configureServer(server) {
      server.middlewares.use(middleware());
    },
    configurePreviewServer(server) {
      server.middlewares.use(middleware());
    },
  };
}
