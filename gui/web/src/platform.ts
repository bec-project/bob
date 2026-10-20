// What the page asks of the desktop app: everything outside OpenCode goes through Rust commands.

declare global {
  interface Window {
    __TAURI_INTERNALS__?: unknown;
  }
}

/** Whether the page runs inside the desktop app; opened in a browser, it can do nothing. */
export function inTauri(): boolean {
  return typeof window !== "undefined" && window.__TAURI_INTERNALS__ !== undefined;
}

export async function tauriInvoke<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<T>(command, args);
}

/** Opens a link in the system browser. */
export async function openExternal(url: string) {
  await tauriInvoke("open_external", { url });
}

/** Saves a text file in the Downloads folder (a numbered name when it exists) and returns its path. */
export function saveTextFile(name: string, text: string): Promise<string> {
  return tauriInvoke<string>("save_download", { name, text });
}

/** A bob container found on Docker or Podman. */
export interface RunningBob {
  url: string;
  password: string;
  container: string;
  runtime: string;
  workspace: string;
  image: string;
  started: string;
}

/**
 * Where the app stands with the user's bob. The app holds one lease at most, in Rust
 * (gui/desktop/src/bob.rs); the page shows it and changes it only by joining and leaving.
 */
export type BobStatus =
  | { state: "idle"; error: string | null }
  | { state: "joining"; config: string }
  /** runningConfig differs from config when another program started bob with another bob.toml. */
  | { state: "joined"; connection: unknown; container: string; config: string; runningConfig: string }
  | { state: "lost"; reason: string; config: string; container: string };

/** How the app starts bob, kept by Rust in the app's config folder; unset paths use the defaults. */
export interface BobSettings {
  config: string | null;
  binary: string | null;
  /** Join (or start) bob when the app opens. */
  autoJoin: boolean;
}

export interface BobSettingsView extends BobSettings {
  /** bob.toml of the source tree the app was built from, if it exists. */
  defaultConfig: string | null;
  /** $BOB_BIN, the source tree's build/bob-manager, or bob on the PATH. */
  defaultBinary: string | null;
}

export function bobSettings(): Promise<BobSettingsView> {
  return tauriInvoke("bob_settings");
}

export function setBobSettings(settings: BobSettings): Promise<BobSettingsView> {
  return tauriInvoke("set_bob_settings", { settings });
}

/** bob containers running on this machine. */
export function listBobs(): Promise<RunningBob[]> {
  return tauriInvoke("list_bobs");
}

export function bobStatus(): Promise<BobStatus> {
  return tauriInvoke("bob_status");
}

/**
 * Joins the user's bob, starting it with the chosen bob.toml when none runs; resolves once bob is
 * up or the join failed. Already holding bob, it resolves with that; during another join, with
 * its end.
 */
export function joinBob(): Promise<BobStatus> {
  return tauriInvoke("bob_join");
}

/** Ends the app's lease on bob (or a join in progress); bob stops once no client holds it. */
export function leaveBob(): Promise<BobStatus> {
  return tauriInvoke("bob_leave");
}

/** Calls handler on every change of the bob status; resolves with the function that stops it. */
export async function onBobStatus(handler: (status: BobStatus) => void): Promise<() => void> {
  const { listen } = await import("@tauri-apps/api/event");
  return listen<BobStatus>("bob-status", (event) => handler(event.payload));
}

/** A native file picker. */
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
