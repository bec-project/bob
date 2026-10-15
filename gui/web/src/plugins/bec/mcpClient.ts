import { loopbackRequest } from "../../platform";
import { SseParser } from "../../api/events";

const PROTOCOL_VERSION = "2025-06-18";

interface JsonRpcResponse {
  jsonrpc: "2.0";
  id?: number | string;
  result?: unknown;
  error?: { code: number; message: string };
}

export interface ToolCallResult {
  content?: { type: string; text?: string }[];
  structuredContent?: unknown;
  isError?: boolean;
}

/** The JSON-RPC response with this id in a JSON or text/event-stream body. */
export function findResponse(body: string, contentType: string, id: number): JsonRpcResponse | undefined {
  const messages: unknown[] = [];
  if (contentType.includes("text/event-stream")) {
    for (const data of new SseParser().push(`${body}\n\n`)) {
      try {
        messages.push(JSON.parse(data));
      } catch {
        // keep-alive comments and partial frames
      }
    }
  } else if (body.trim()) {
    const value = JSON.parse(body) as unknown;
    messages.push(...(Array.isArray(value) ? value : [value]));
  }
  return messages.find((m): m is JsonRpcResponse => (m as JsonRpcResponse)?.id === id);
}

/** Where the host reaches an MCP URL written for the container (host.docker.internal etc.). */
export function hostUrl(containerUrl: string): string {
  try {
    const url = new URL(containerUrl);
    if (["host.docker.internal", "host.containers.internal", "gateway.docker.internal"].includes(url.hostname)) {
      url.hostname = "127.0.0.1";
    }
    return url.toString();
  } catch {
    return containerUrl;
  }
}

/**
 * A minimal client for an MCP server's streamable-HTTP transport: initialize once, then call
 * tools. The GUI uses it only for the BEC panels, never for the model's own tool calls.
 */
export class McpClient {
  private sessionID?: string;
  private ready?: Promise<void>;
  private nextID = 1;

  constructor(
    readonly url: string,
    private readonly token?: string,
  ) {}

  private headers(): Record<string, string> {
    const headers: Record<string, string> = {
      "content-type": "application/json",
      accept: "application/json, text/event-stream",
      "mcp-protocol-version": PROTOCOL_VERSION,
    };
    if (this.sessionID) headers["mcp-session-id"] = this.sessionID;
    if (this.token) headers.authorization = `Bearer ${this.token}`;
    return headers;
  }

  private async rpc(method: string, params: unknown, notify = false): Promise<unknown> {
    const id = this.nextID++;
    const message = notify ? { jsonrpc: "2.0", method, params } : { jsonrpc: "2.0", id, method, params };
    const response = await loopbackRequest({
      url: this.url,
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(message),
    });
    if (response.status === 404 && this.sessionID) {
      throw new SessionExpired();
    }
    if (response.status >= 400) {
      const detail = response.body.slice(0, 300).trim();
      throw new Error(`${method}: HTTP ${response.status}${detail ? ` (${detail})` : ""}`);
    }
    const sessionID = response.headers["mcp-session-id"];
    if (sessionID) this.sessionID = sessionID;
    if (notify) return undefined;
    const reply = findResponse(response.body, response.headers["content-type"] ?? "", id);
    if (!reply) throw new Error(`${method}: the server sent no answer`);
    if (reply.error) throw new Error(`${method}: ${reply.error.message}`);
    return reply.result;
  }

  private connect(): Promise<void> {
    this.ready ??= (async () => {
      await this.rpc("initialize", {
        protocolVersion: PROTOCOL_VERSION,
        capabilities: {},
        clientInfo: { name: "bob-gui", version: "0.1.0" },
      });
      await this.rpc("notifications/initialized", {}, true);
    })().catch((error: unknown) => {
      this.ready = undefined;
      throw error;
    });
    return this.ready;
  }

  /** The server's tools (first page; the BEC servers list all of theirs at once). */
  async listTools(): Promise<{ name: string; description?: string }[]> {
    await this.connect();
    const result = (await this.rpc("tools/list", {})) as { tools?: { name: string; description?: string }[] };
    return result.tools ?? [];
  }

  /** Calls a tool and returns its structured result (or its text parsed as JSON). */
  async callTool<T = unknown>(name: string, args: Record<string, unknown> = {}): Promise<T> {
    for (let attempt = 0; ; attempt++) {
      await this.connect();
      try {
        const result = (await this.rpc("tools/call", { name, arguments: args })) as ToolCallResult;
        const textContent = result.content?.find((c) => c.type === "text")?.text ?? "";
        if (result.isError) throw new Error(textContent || `${name} failed`);
        if (result.structuredContent !== undefined) return result.structuredContent as T;
        try {
          return JSON.parse(textContent) as T;
        } catch {
          return textContent as T;
        }
      } catch (error) {
        if (error instanceof SessionExpired && attempt === 0) {
          this.sessionID = undefined;
          this.ready = undefined;
          continue;
        }
        throw error;
      }
    }
  }
}

class SessionExpired extends Error {
  constructor() {
    super("The MCP session expired.");
  }
}
