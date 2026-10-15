import { useCallback, useEffect, useState } from "react";
import { bobConnection, parseConnection } from "../api/connection";
import { bobDefaults, inTauri, listBobs, pickFile, type RunningBob } from "../platform";
import { useStore } from "../state/store";
import { relativeTime } from "./hooks";
import { IconLogo, IconRefresh } from "./Icons";

/** Desktop app: join (or start) the user's shared bob, or connect to another running bob. */
function DesktopBobs() {
  const connect = useStore((s) => s.connect);
  const startBob = useStore((s) => s.startBob);
  const prefs = useStore((s) => s.prefs);
  const setPrefs = useStore((s) => s.setPrefs);
  const [running, setRunning] = useState<RunningBob[]>();
  const [defaults, setDefaults] = useState<{ binary: string | null; config: string | null }>();
  const [listing, setListing] = useState(false);

  const refresh = useCallback(async () => {
    setListing(true);
    setRunning(await listBobs().catch(() => []));
    setListing(false);
  }, []);

  useEffect(() => {
    void refresh();
    void bobDefaults()
      .then(setDefaults)
      .catch(() => setDefaults({ binary: null, config: null }));
  }, [refresh]);

  const config = prefs.bobConfig ?? defaults?.config ?? "";
  const binary = prefs.bobBinary ?? defaults?.binary ?? "";

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
        <label htmlFor="bob-config">bob.toml</label>
        <div className="path-row">
          <input
            id="bob-config"
            className="input"
            placeholder="/path/to/bob.toml"
            value={config}
            onChange={(event) => setPrefs({ bobConfig: event.target.value })}
          />
          <button
            className="btn"
            onClick={() =>
              void pickFile("Choose a bob.toml", ["toml"]).then((path) => path && setPrefs({ bobConfig: path }))
            }
          >
            Choose…
          </button>
        </div>
        <label htmlFor="bob-binary">bob executable</label>
        <div className="path-row">
          <input
            id="bob-binary"
            className="input"
            placeholder="build/bob-manager, or bob on the PATH"
            value={binary}
            onChange={(event) => setPrefs({ bobBinary: event.target.value })}
          />
          <button
            className="btn"
            onClick={() => void pickFile("Choose the bob executable").then((path) => path && setPrefs({ bobBinary: path }))}
          >
            Choose…
          </button>
        </div>
        <div className="connect-actions">
          <label className="toggle">
            <input
              type="checkbox"
              checked={prefs.autoStartBob}
              onChange={(event) => setPrefs({ autoStartBob: event.target.checked })}
            />
            <span className="track" />
            Do this when the app opens
          </label>
          <button
            className="btn primary"
            disabled={!config || !binary}
            onClick={() => {
              setPrefs({ bobConfig: config, bobBinary: binary });
              void startBob(binary, config);
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
        {running?.map((bob) => (
          <div key={bob.container} className="bob-row">
            <div className="grow">
              <div className="name">
                {bob.container}
                {bob.owned_config && <span className="chip">yours</span>}
              </div>
              <div className="muted" title={bob.workspace}>
                {bob.workspace || "?"} · {bob.runtime} · started {relativeTime(Date.parse(bob.started))} ago
              </div>
            </div>
            <button className="btn small" onClick={() => void connect(bobConnection(bob))}>
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
  const desktop = inTauri();
  const [text, setText] = useState("");
  const [password, setPassword] = useState("");
  const [manual, setManual] = useState(!desktop);
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
        {desktop && <DesktopBobs />}

        {desktop && !manual ? (
          <button className="btn ghost small" style={{ marginTop: 12 }} onClick={() => setManual(true)}>
            Connect by hand…
          </button>
        ) : (
          <form
            className={desktop ? "connect-section" : ""}
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
              autoFocus={!desktop}
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
                Paste the JSON line bob printed, or a URL and its password.
              </span>
              <button className="btn primary" type="submit" disabled={!ready || phase === "connecting"}>
                {phase === "connecting" ? "Connecting…" : "Connect"}
              </button>
            </div>
          </form>
        )}
        {!desktop && (
          <div className="connect-hint">
            Start bob and the GUI together from the repository root, and the GUI connects by itself:
            <pre style={{ marginTop: 8 }}>
              <code>npm --prefix gui/web run bob -- --config bob.toml</code>
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}
