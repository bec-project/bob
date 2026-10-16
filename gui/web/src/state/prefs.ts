import type { ModelRef } from "../api/types";

export type Theme = "system" | "light" | "dark";

/** Per-user choices, kept in localStorage. Nothing secret goes here. */
export interface Prefs {
  theme: Theme;
  /** Plugin id to on/off; plugins not listed use their default. */
  plugins: Record<string, boolean>;
  pluginSettings: Record<string, Record<string, unknown>>;
  model?: ModelRef;
  /** Whether to let the model think before answering, where the model can switch it off. */
  thinking: boolean;
  agent?: string;
  /** The side panel that is open, as "<plugin>/<panel>". */
  panel?: string;
  sidebar: boolean;
  /** Desktop app: the bob.toml to start bob with, and the bob executable. */
  bobConfig?: string;
  bobBinary?: string;
  /** Desktop app: start bob on launch when none is running. */
  autoStartBob: boolean;
}

const KEY = "bob-gui.prefs";

export const defaultPrefs: Prefs = {
  theme: "system",
  plugins: {},
  pluginSettings: {},
  thinking: true,
  sidebar: true,
  autoStartBob: true,
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
