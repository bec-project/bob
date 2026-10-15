import { useMemo } from "react";
import type { PluginContext } from "../types";
import { parseToolResult, type ToolResult } from "./cards";
import { hostUrl, McpClient } from "./mcpClient";

const TOKEN_KEY = "bob-gui.bec.mcp-token";
const clients = new Map<string, McpClient>();
const toolLists = new Map<McpClient, Promise<Set<string>>>();

export interface BecServer {
  /** The server's name in bob.toml ([mcp.<name>]), or "settings" for a URL set by hand. */
  name: string;
  /** Where this machine reaches it. */
  url: string;
  source: string;
}

export function mcpClient(url: string, token: string): McpClient {
  const key = `${url}\n${token}`;
  let value = clients.get(key);
  if (!value) {
    value = new McpClient(url, token || undefined);
    clients.set(key, value);
  }
  return value;
}

export function readToken(): string {
  try {
    return sessionStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

export function writeToken(token: string) {
  try {
    if (token) sessionStorage.setItem(TOKEN_KEY, token);
    else sessionStorage.removeItem(TOKEN_KEY);
  } catch {
    // storage blocked: the token lasts until reload
  }
}

/**
 * The MCP servers the BEC panels may talk to: the URL set in Settings, or every remote server in
 * OpenCode's configuration (BEC-named ones first), rewritten from the container's view of the
 * host to loopback.
 */
export function becServers(context: PluginContext): BecServer[] {
  const configured = String(context.settings.mcpUrl ?? "").trim();
  if (configured) return [{ name: "settings", url: configured, source: "Settings" }];
  return Object.entries(context.mcpServers)
    .filter(([, entry]) => entry.url && entry.enabled !== false)
    .sort(([a], [b]) => Number(/bec/i.test(b)) - Number(/bec/i.test(a)))
    .map(([name, entry]) => ({ name, url: hostUrl(entry.url!), source: `bob.toml [mcp.${name}]` }));
}

/** The server the panels read from: the first of becServers. */
export function becServerUrl(context: PluginContext): { url?: string; source: string } {
  const first = becServers(context)[0];
  return first ? { url: first.url, source: first.source } : { source: "none" };
}

/**
 * The server behind an OpenCode tool id. OpenCode names MCP tools "<server>_<tool>", so the
 * longest configured server name that prefixes the id wins.
 */
export function serverForToolId(context: PluginContext, toolID: string): BecServer | undefined {
  const configured = becServers(context);
  if (configured.length === 1 && configured[0].name === "settings") return configured[0];
  return configured
    .filter((server) => toolID.startsWith(`${server.name}_`))
    .sort((a, b) => b.name.length - a.name.length)[0];
}

/** The tools a server offers, asked once per client. */
export function serverTools(client: McpClient): Promise<Set<string>> {
  let list = toolLists.get(client);
  if (!list) {
    list = client.listTools().then((tools) => new Set(tools.map((tool) => tool.name)));
    list.catch(() => toolLists.delete(client));
    toolLists.set(client, list);
  }
  return list;
}

/** The first configured server that offers a tool, e.g. a DEV-mode server for bec_queue_control. */
export async function serverWithTool(context: PluginContext, tool: string): Promise<McpClient | undefined> {
  const token = readToken();
  for (const server of becServers(context)) {
    const client = mcpClient(server.url, token);
    const tools = await serverTools(client).catch(() => new Set<string>());
    if (tools.has(tool)) return client;
  }
  return undefined;
}

/** Calls a BEC tool and returns its tool_result() object. */
export async function callResult(client: McpClient, tool: string, args: Record<string, unknown> = {}): Promise<ToolResult> {
  const value = await client.callTool(tool, args);
  const result = parseToolResult(value);
  if (!result) throw new Error(`${tool} returned an unexpected result`);
  return result;
}

/** The client for the panels' server, or undefined when none is configured. */
export function useBecClient(context: PluginContext): { client?: McpClient; url?: string; source: string } {
  const { url, source } = becServerUrl(context);
  const token = readToken();
  const client = useMemo(() => (url ? mcpClient(url, token) : undefined), [url, token]);
  return { client, url, source };
}
