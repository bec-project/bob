import { useEffect, useState } from "react";
import { inTauri, listBobs, stopBob } from "../platform";
import { allPlugins, usePluginContext } from "../plugins/registry";
import type { GuiPlugin } from "../plugins/types";
import type { Theme } from "../state/prefs";
import { pluginEnabled, useStore } from "../state/store";
import { IconX } from "./Icons";

function PluginRow({ plugin }: { plugin: GuiPlugin }) {
  const prefs = useStore((s) => s.prefs);
  const setPrefs = useStore((s) => s.setPrefs);
  const context = usePluginContext(plugin);
  const on = pluginEnabled(prefs, plugin.id, plugin.defaultEnabled);
  return (
    <div className="setting-row">
      <div className="grow">
        <div style={{ fontWeight: 500 }}>{plugin.name}</div>
        <div className="desc">{plugin.description}</div>
        {on && plugin.settingsView?.(context)}
      </div>
      <label className="toggle">
        <input
          type="checkbox"
          checked={on}
          onChange={(event) => setPrefs({ plugins: { ...prefs.plugins, [plugin.id]: event.target.checked } })}
        />
        <span className="track" />
      </label>
    </div>
  );
}

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const theme = useStore((s) => s.prefs.theme);
  const setPrefs = useStore((s) => s.setPrefs);
  const connection = useStore((s) => s.connection);
  const version = useStore((s) => s.serverVersion);
  const mcpStatus = useStore((s) => s.mcpStatus);
  const disconnect = useStore((s) => s.disconnect);

  const [owned, setOwned] = useState<string>();
  const [stopping, setStopping] = useState(false);
  useEffect(() => {
    if (!inTauri()) return;
    void listBobs()
      .then((bobs) => setOwned(bobs.find((b) => b.container === connection?.container)?.owned_config ?? undefined))
      .catch(() => undefined);
  }, [connection?.container]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="dialog" role="dialog" aria-label="Settings">
        <div className="dialog-head">
          <span className="grow">Settings</span>
          <button className="icon-btn" onClick={onClose} title="Close">
            <IconX size={16} />
          </button>
        </div>
        <section>
          <h3>Appearance</h3>
          <div className="segmented">
            {(["system", "light", "dark"] as Theme[]).map((value) => (
              <button key={value} className={theme === value ? "on" : ""} onClick={() => setPrefs({ theme: value })}>
                {value[0].toUpperCase() + value.slice(1)}
              </button>
            ))}
          </div>
        </section>
        <section>
          <h3>Plugins</h3>
          {allPlugins.map((plugin) => (
            <PluginRow key={plugin.id} plugin={plugin} />
          ))}
        </section>
        <section>
          <h3>Connection</h3>
          <dl className="kv">
            <dt>Server</dt>
            <dd>{connection?.url}</dd>
            {connection?.container && (
              <>
                <dt>Container</dt>
                <dd>
                  {connection.container}
                  {connection.runtime ? ` (${connection.runtime})` : ""}
                </dd>
              </>
            )}
            {connection?.workspace && (
              <>
                <dt>Workspace</dt>
                <dd>{connection.workspace}</dd>
              </>
            )}
            {version && (
              <>
                <dt>OpenCode</dt>
                <dd>{version}</dd>
              </>
            )}
            {Object.entries(mcpStatus).map(([name, status]) => (
              <span key={name} style={{ display: "contents" }}>
                <dt>MCP {name}</dt>
                <dd className={status.status === "connected" ? "tone-ok" : "tone-warn"}>
                  {status.status}
                  {status.error ? `: ${status.error}` : ""}
                </dd>
              </span>
            ))}
          </dl>
          <div style={{ marginTop: 12, display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button
              className="btn"
              onClick={() => {
                onClose();
                disconnect();
              }}
            >
              {inTauri() ? "Switch bob…" : "Disconnect"}
            </button>
            {owned && (
              <button
                className="btn danger"
                disabled={stopping}
                title={`End this app's hold on ${connection?.container}; bob stops when no other app uses it, and its chats stay in the home volume`}
                onClick={() => {
                  setStopping(true);
                  void stopBob(connection?.container).then(() => {
                    setStopping(false);
                    onClose();
                    disconnect();
                  });
                }}
              >
                {stopping ? "Leaving…" : "Leave this bob"}
              </button>
            )}
          </div>
          {owned && (
            <div className="desc" style={{ marginTop: 8 }}>
              This app holds your bob ({owned}); quitting or leaving ends its hold, and bob stops once no app
              holds it.
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
