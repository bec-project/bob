import type { RunningBob } from "../platform";
import type { BobConnection } from "./types";

/**
 * Reads what the user pasted: bob's connection JSON, or a bare URL followed by the password.
 * Returns null when it is neither.
 */
export function parseConnection(text: string, password = ""): BobConnection | null {
  const trimmed = text.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith("{")) {
    try {
      return toConnection(JSON.parse(trimmed));
    } catch {
      return null;
    }
  }
  try {
    const url = new URL(trimmed);
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    return { url: url.origin + url.pathname.replace(/\/+$/, ""), password };
  } catch {
    return null;
  }
}

/** bob's connection object (as the broker sends it), or null when it lacks the URL or password. */
export function toConnection(value: unknown): BobConnection | null {
  const object = value as Partial<BobConnection> | null;
  if (typeof object !== "object" || object === null) return null;
  if (typeof object.url !== "string" || typeof object.password !== "string") return null;
  return { ...object, url: object.url, password: object.password };
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
