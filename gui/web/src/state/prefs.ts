import type { ModelRef } from "../api/types";

export type Theme = "system" | "light" | "dark";

/** Per-user choices about the page, kept in localStorage. Nothing secret goes here; how bob
 * starts is the desktop app's own setting (see BobSettings). */
export interface Prefs {
  theme: Theme;
  model?: ModelRef;
  /** Whether to let the model think before answering, where the model can switch it off. */
  thinking: boolean;
  agent?: string;
  /** Widths set by dragging a splitter, by element ("sidebar", or a side panel's key). */
  widths: Record<string, number>;
  /** The side panel that is open. */
  panel?: string;
  sidebar: boolean;
}

const KEY = "bob-gui.prefs";

export const defaultPrefs: Prefs = {
  theme: "system",
  thinking: true,
  sidebar: true,
  widths: {},
};

export function loadPrefs(): Prefs {
  try {
    const text = localStorage.getItem(KEY);
    if (!text) return defaultPrefs;
    return { ...defaultPrefs, ...(JSON.parse(text) as Partial<Prefs>) };
  } catch {
    return defaultPrefs;
  }
}

export function savePrefs(prefs: Prefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(prefs));
  } catch {
    // storage blocked: choices last until the window closes
  }
}
