import { useCallback, useEffect, useState } from "react";
import { bobConnection, parseConnection } from "../api/connection";
import { bobSettings, listBobs, pickFile, setBobSettings, type BobSettingsView, type RunningBob } from "../platform";
import { useStore } from "../state/store";
import { relativeTime } from "./hooks";
import { IconLogo, IconRefresh } from "./Icons";

/** Join (or start) the user's bob, or connect to another running bob. */
function DesktopBobs() {
  const connect = useStore((s) => s.connect);
  const joinBob = useStore((s) => s.joinBob);
  const leaveBob = useStore((s) => s.leaveBob);
  const bob = useStore((s) => s.bob);
  const [running, setRunning] = useState<RunningBob[]>();
  const [listing, setListing] = useState(false);
  // how the app starts bob, kept by Rust; empty fields use the defaults shown as placeholders
  const [settings, setSettings] = useState<BobSettingsView>();
  const [config, setConfig] = useState("");
  const [binary, setBinary] = useState("");
  const [saveError, setSaveError] = useState<string>();

  const refresh = useCallback(async () => {
    setListing(true);
    setRunning(await listBobs().catch(() => []));
    setListing(false);
  }, []);

  useEffect(() => {
    void refresh();
    void bobSettings()
      .then((view) => {
        setSettings(view);
        setConfig(view.config ?? "");
        setBinary(view.binary ?? "");
      })
      .catch((error: Error) => setSaveError(error.message));
  }, [refresh]);

  const save = async (autoJoin: boolean) => {
    try {
      setSettings(await setBobSettings({ config: config.trim() || null, binary: binary.trim() || null, autoJoin }));
      setSaveError(undefined);
      return true;
    } catch (error) {
      setSaveError((error as Error).message);
      return false;
    }
  };
  const ready = Boolean(config.trim() || settings?.defaultConfig) && Boolean(binary.trim() || settings?.defaultBinary);

  return (
    <>
      <div className="connect-section">
        <div className="connect-section-head">
          <span>Your bob</span>
        </div>
        <div className="muted" style={{ fontSize: 12.5, marginBottom: 4 }}>
          One bob per user, shared by every app that uses it: this joins it, or starts it with your bob.toml
          when none runs. It stops 90 seconds after the last app leaves.
        </div>
        {bob.state === "joined" && (
          <div className="bob-row">
            <div className="grow">
              This app holds <span className="name">{bob.container}</span>.
            </div>
            <button className="btn small" onClick={() => void leaveBob()}>
              Leave it
            </button>
          </div>
        )}
        <label htmlFor="bob-config">bob.toml</label>
        <div className="path-row">
          <input
            id="bob-config"
            className="input"
            placeholder={settings?.defaultConfig ?? "/path/to/bob.toml"}
            value={config}
            onChange={(event) => setConfig(event.target.value)}
          />
          <button
            className="btn"
            onClick={() => void pickFile("Choose a bob.toml", ["toml"]).then((path) => path && setConfig(path))}
          >
            Choose…
          </button>
        </div>
        <label htmlFor="bob-binary">bob executable</label>
        <div className="path-row">
          <input
            id="bob-binary"
            className="input"
            placeholder={settings?.defaultBinary ?? "build/bob-manager, or bob on the PATH"}
            value={binary}
            onChange={(event) => setBinary(event.target.value)}
          />
          <button
            className="btn"
            onClick={() => void pickFile("Choose the bob executable").then((path) => path && setBinary(path))}
          >
            Choose…
          </button>
        </div>
        {saveError && <div className="tone-error" style={{ fontSize: 12.5 }}>{saveError}</div>}
        <div className="connect-actions">
          <label className="toggle">
            <input
              type="checkbox"
              checked={settings?.autoJoin ?? true}
              disabled={!settings}
              onChange={(event) => void save(event.target.checked)}
            />
            <span className="track" />
            Do this when the app opens
          </label>
          <button
            className="btn primary"
            disabled={!settings || !ready}
            onClick={() => {
              void save(settings?.autoJoin ?? true).then((saved) => {
                if (saved) void joinBob();
              });
            }}
          >
            Join or start bob
          </button>
        </div>
      </div>

      <div className="connect-section">
        <div className="connect-section-head">
          <span>Running bobs</span>
          <button className="icon-btn" title="Look again" onClick={() => void refresh()} disabled={listing}>
            {listing ? <span className="spinner" /> : <IconRefresh size={14} />}
          </button>
        </div>
        {running?.length === 0 && <div className="muted">No bob container runs on Docker or Podman.</div>}
        {running?.map((found) => (
          <div key={found.container} className="bob-row">
            <div className="grow">
              <div className="name">
                {found.container}
                {bob.state === "joined" && bob.container === found.container && <span className="chip">held</span>}
              </div>
              <div className="muted" title={found.workspace}>
                {found.workspace || "?"} · {found.runtime} · started {relativeTime(Date.parse(found.started))} ago
              </div>
            </div>
            <button className="btn small" onClick={() => void connect(bobConnection(found))}>
              Connect
            </button>
          </div>
        ))}
        {running && running.length > 0 && (
          <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
            Connecting here does not hold the bob: it keeps the lifetime it was started with.
          </div>
        )}
      </div>
    </>
  );
}

export function ConnectScreen() {
  const phase = useStore((s) => s.phase);
  const error = useStore((s) => s.connectError);
  const connect = useStore((s) => s.connect);
  const [text, setText] = useState("");
  const [password, setPassword] = useState("");
  const [manual, setManual] = useState(false);
  const looksLikeUrl = /^https?:\/\//i.test(text.trim());
  const connection = parseConnection(text, password);
  const ready = connection !== null && (!looksLikeUrl || password.length > 0);

  return (
    <div className="connect">
      <div className="connect-card">
        <h1>
          <IconLogo size={26} /> bob
        </h1>
        <p>Connect to the OpenCode server that bob runs in its container.</p>
        {error && <pre className="connect-error">{error}</pre>}
        <DesktopBobs />

        {!manual ? (
          <button className="btn ghost small" style={{ marginTop: 12 }} onClick={() => setManual(true)}>
            Connect by hand…
          </button>
        ) : (
          <form
            className="connect-section"
            onSubmit={(event) => {
              event.preventDefault();
              if (connection && ready) void connect(connection);
            }}
          >
            <label htmlFor="connection">Connection</label>
            <textarea
              id="connection"
              className="input"
              rows={4}
              spellCheck={false}
              placeholder={'{"url":"http://127.0.0.1:52997","password":"…","container":"bob-…"}'}
              value={text}
              onChange={(event) => setText(event.target.value)}
              style={{ fontFamily: "var(--mono)", fontSize: 12.5 }}
            />
            {looksLikeUrl && (
              <>
                <label htmlFor="password">Password</label>
                <input
                  id="password"
                  className="input"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </>
            )}
            <div className="connect-actions">
              <span className="muted" style={{ fontSize: 12.5 }}>
                Paste a connection (JSON with url and password), or a URL and its password.
              </span>
              <button className="btn primary" type="submit" disabled={!ready || phase === "connecting"}>
                {phase === "connecting" ? "Connecting…" : "Connect"}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
