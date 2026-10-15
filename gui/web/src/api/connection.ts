import { inTauri, tauriInvoke, type RunningBob } from "../platform";
import type { BobConnection } from "./types";

const STORAGE_KEY = "bob-gui.connection";

/**
 * Reads what the user pasted: bob's connection JSON, or a bare URL followed by the password.
 * Returns null when it is neither.
 */
export function parseConnection(text: string, password = ""): BobConnection | null {
  const trimmed = text.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith("{")) {
    try {
      const value = JSON.parse(trimmed) as Partial<BobConnection>;
      if (typeof value.url === "string" && typeof value.password === "string") {
        return { ...value, url: value.url, password: value.password };
      }
    } catch {
      return null;
    }
    return null;
  }
  try {
    const url = new URL(trimmed);
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    return { url: url.origin + url.pathname.replace(/\/+$/, ""), password };
  } catch {
    return null;
  }
}

/** The connection bob's launcher handed over, if the GUI was started through it. */
export async function launcherConnection(): Promise<BobConnection | null> {
  try {
    if (inTauri()) {
      const text = await tauriInvoke<string | null>("bob_connection");
      return text ? parseConnection(text) : null;
    }
    const response = await fetch("/__bob/connection", { headers: { "x-bob-gui": "1" } });
    if (response.status !== 200) return null;
    return parseConnection(await response.text());
  } catch {
    return null;
  }
}

/**
 * The last connection, kept for this browser tab only: bob generates a new password on every
 * launch, so persisting it on disk would only keep a stale credential around.
 */
export function savedConnection(): BobConnection | null {
  try {
    const text = sessionStorage.getItem(STORAGE_KEY);
    return text ? parseConnection(text) : null;
  } catch {
    return null;
  }
}

export function saveConnection(connection: BobConnection | null) {
  try {
    if (connection) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(connection));
    else sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // storage blocked: the user pastes the connection again next time
  }
}

/** The connection to a bob container the desktop app found. */
export function bobConnection(bob: RunningBob): BobConnection {
  return {
    url: bob.url,
    password: bob.password,
    container: bob.container,
    runtime: bob.runtime,
    workspace: bob.workspace,
  };
}
