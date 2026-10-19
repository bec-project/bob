// The few places where the browser build and the Tauri desktop build differ.

declare global {
  interface Window {
    __TAURI_INTERNALS__?: unknown;
  }
}

export function inTauri(): boolean {
  return typeof window !== "undefined" && window.__TAURI_INTERNALS__ !== undefined;
}

export async function tauriInvoke<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<T>(command, args);
}

export interface RelayRequest {
  url: string;
  method: "GET" | "POST" | "DELETE";
  headers: Record<string, string>;
  body?: string;
}

export interface RelayResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
}

/**
 * Sends one HTTP request to a server on this machine's loopback interface that a page cannot
 * call directly because it sends no CORS headers (the BEC MCP server). The dev server relays it
 * in the browser build, a Rust command in the desktop build.
 */
export async function loopbackRequest(request: RelayRequest): Promise<RelayResponse> {
  if (inTauri()) return tauriInvoke<RelayResponse>("loopback_request", { request });
  const response = await fetch("/__bob/mcp", {
    method: request.method,
    headers: { ...request.headers, "x-bob-gui": "1", "x-bob-target": request.url },
    body: request.body,
  });
  const headers: Record<string, string> = {};
  response.headers.forEach((value, name) => {
    headers[name] = value;
  });
  return { status: response.status, headers, body: await response.text() };
}

/** Opens a link in the system browser (desktop) or a new tab (browser). */
export async function openExternal(url: string) {
  if (inTauri()) {
    await tauriInvoke("open_external", { url });
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

/**
 * Saves a text file for the user: a browser download, or in the desktop app a file in the
 * Downloads folder (a numbered name when it exists). Returns where it went, when known.
 */
export async function saveTextFile(name: string, text: string, type = "text/markdown"): Promise<string | undefined> {
  if (inTauri()) return tauriInvoke<string>("save_download", { name, text });
  const url = URL.createObjectURL(new Blob([text], { type: `${type};charset=utf-8` }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  return undefined;
}

/** A bob container the desktop app found on Docker or Podman. */
export interface RunningBob {
  url: string;
  password: string;
  container: string;
  runtime: string;
  workspace: string;
  image: string;
  started: string;
  /** Set when this app started it: the bob.toml it was started with. */
  owned_config: string | null;
}

/** Where bob and a bob.toml probably are (desktop app only). */
export function bobDefaults(): Promise<{ binary: string | null; config: string | null }> {
  return tauriInvoke("bob_defaults");
}

/** bob containers running on this machine (desktop app only). */
export function listBobs(): Promise<RunningBob[]> {
  return tauriInvoke("list_bobs");
}

/** Starts bob with a bob.toml and resolves with its connection line (desktop app only). */
export function startBob(binary: string, config: string): Promise<string> {
  return tauriInvoke("start_bob", { binary, config });
}

/** Stops a bob this app started, or all of them; resolves with the ones that did not stop. */
export function stopBob(container?: string): Promise<string[]> {
  return tauriInvoke("stop_bob", { container: container ?? null });
}

/** A native file picker (desktop app only). */
export async function pickFile(title: string, extensions?: string[]): Promise<string | null> {
  const { open } = await import("@tauri-apps/plugin-dialog");
  const result = await open({
    title,
    multiple: false,
    directory: false,
    filters: extensions ? [{ name: extensions.join(", "), extensions }] : undefined,
  });
  return typeof result === "string" ? result : null;
}
