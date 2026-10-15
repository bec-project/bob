import { useMemo } from "react";
import { useStore } from "../state/store";
import { becPlugin } from "./bec";
import type { PermissionRequest, ToolPart } from "../api/types";
import type { GuiPlugin, PermissionView, PluginContext } from "./types";

/** Every plugin the GUI ships. Add new ones here. */
export const allPlugins: GuiPlugin[] = [becPlugin];

export function useEnabledPlugins(): GuiPlugin[] {
  const plugins = useStore((s) => s.prefs.plugins);
  return useMemo(
    () => allPlugins.filter((p) => plugins[p.id] ?? p.defaultEnabled),
    [plugins],
  );
}

/** The context handed to one plugin's views. */
export function usePluginContext(plugin: GuiPlugin): PluginContext {
  const settings = useStore((s) => s.prefs.pluginSettings[plugin.id]);
  const mcpServers = useStore((s) => s.mcpConfig);
  return useMemo<PluginContext>(
    () => ({
      insertPrompt: (text) => useStore.getState().insertPrompt(text),
      sendPrompt: (text) => void useStore.getState().send(text),
      mcpServers,
      settings: settings ?? {},
      setSettings(patch) {
        const state = useStore.getState();
        const all = state.prefs.pluginSettings;
        state.setPrefs({ pluginSettings: { ...all, [plugin.id]: { ...all[plugin.id], ...patch } } });
      },
    }),
    [plugin.id, settings, mcpServers],
  );
}

/** A context outside React (for describe* calls made while rendering lists). */
export function pluginContext(plugin: GuiPlugin): PluginContext {
  const state = useStore.getState();
  return {
    insertPrompt: (text) => state.insertPrompt(text),
    sendPrompt: (text) => void useStore.getState().send(text),
    mcpServers: state.mcpConfig,
    settings: state.prefs.pluginSettings[plugin.id] ?? {},
    setSettings(patch) {
      const current = useStore.getState();
      const all = current.prefs.pluginSettings;
      current.setPrefs({ pluginSettings: { ...all, [plugin.id]: { ...all[plugin.id], ...patch } } });
    },
  };
}

/** Plugins that can describe a permission request, in order; used by the request cards. */
export function permissionView(
  request: PermissionRequest,
  tool: ToolPart | undefined,
  enabled: string[],
): PermissionView | undefined {
  for (const plugin of allPlugins) {
    if (!enabled.includes(plugin.id)) continue;
    const view = plugin.describePermission?.(request, tool, pluginContext(plugin));
    if (view) return view;
  }
  return undefined;
}

