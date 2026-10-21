import { useEffect } from "react";
import type { Theme } from "../state/prefs";
import { useStore } from "../state/store";
import { IconX } from "./Icons";

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const theme = useStore((s) => s.prefs.theme);
  const setPrefs = useStore((s) => s.setPrefs);
  const connection = useStore((s) => s.connection);
  const version = useStore((s) => s.serverVersion);
  const mcpStatus = useStore((s) => s.mcpStatus);
  const bob = useStore((s) => s.bob);
  const leaveBob = useStore((s) => s.leaveBob);
  const held = bob.state === "joined" && bob.container === connection?.container ? bob : undefined;

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
              className={held ? "btn danger" : "btn"}
              title={held ? `End this app's hold on ${held.container}; its chats stay in the home volume` : undefined}
              onClick={() => {
                onClose();
                void leaveBob();
              }}
            >
              {held ? "Leave this bob" : "Switch bob…"}
            </button>
          </div>
          {held && (
            <div className="desc" style={{ marginTop: 8 }}>
              This app holds your bob, started with {held.runningConfig}. Leaving ends its hold and goes back
              to the connect screen; bob stops 90 seconds after no app holds it.
              {held.runningConfig !== held.config && (
                <> This app asked for {held.config}, but another program had started bob first.</>
              )}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
