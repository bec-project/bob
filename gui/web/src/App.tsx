import { useEffect, useState } from "react";
import { bobConnection, launcherConnection, savedConnection } from "./api/connection";
import { bobDefaults, inTauri, listBobs } from "./platform";
import { Composer } from "./components/Composer";
import { ConnectScreen } from "./components/ConnectScreen";
import { Header, UsageBar } from "./components/Header";
import { Requests } from "./components/Requests";
import { SettingsDialog } from "./components/SettingsDialog";
import { Sidebar } from "./components/Sidebar";
import { SidePanels } from "./components/SidePanels";
import { Thread } from "./components/Thread";
import { Todos } from "./components/Todos";
import { useStore } from "./state/store";

function useTheme() {
  const theme = useStore((s) => s.prefs.theme);
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      const dark = theme === "dark" || (theme === "system" && media.matches);
      document.documentElement.dataset.theme = dark ? "dark" : "light";
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [theme]);
}

/**
 * Connect on start: to the launcher's bob; in the desktop app to the user's bob (joined,
 * or started from the remembered bob.toml), or to the one running bob when no bob.toml is set;
 * else to the one used last.
 */
function useAutoConnect() {
  const [checked, setChecked] = useState(false);
  const connect = useStore((s) => s.connect);
  const startBob = useStore((s) => s.startBob);
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const launched = await launcherConnection();
      if (cancelled) return;
      if (launched) {
        await connect(launched);
      } else if (inTauri()) {
        const prefs = useStore.getState().prefs;
        const binary = prefs.bobBinary || (await bobDefaults().catch(() => ({ binary: null }))).binary;
        if (cancelled) return;
        if (prefs.autoStartBob && prefs.bobConfig && binary) {
          // joins the bob this user runs, or starts it
          await startBob(binary, prefs.bobConfig);
        } else {
          const running = await listBobs().catch(() => []);
          if (running.length === 1 && !cancelled) await connect(bobConnection(running[0]));
        }
      } else {
        const saved = savedConnection();
        if (saved) await connect(saved);
      }
      setChecked(true);
    })();
    return () => {
      cancelled = true;
    };
  }, [connect, startBob]);
  return checked;
}

function Shortcuts({ onSettings }: { onSettings: () => void }) {
  const select = useStore((s) => s.selectSession);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const mod = event.metaKey || event.ctrlKey;
      if (mod && event.shiftKey && event.key.toLowerCase() === "o") {
        event.preventDefault();
        select(undefined);
      } else if (mod && event.key === ",") {
        event.preventDefault();
        onSettings();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [select, onSettings]);
  return null;
}

export function App() {
  useTheme();
  const checked = useAutoConnect();
  const phase = useStore((s) => s.phase);
  const sidebar = useStore((s) => s.prefs.sidebar);
  const starting = useStore((s) => s.starting);
  const [settings, setSettings] = useState(false);

  if (phase !== "ready") {
    if (phase === "starting") {
      return (
        <div className="connect">
          <div className="working">
            <span className="spinner" /> Joining your bob, or starting it with {starting?.split("/").pop()}…
          </div>
        </div>
      );
    }
    if (!checked || phase === "connecting") {
      return (
        <div className="connect">
          <div className="working">
            <span className="spinner" /> Connecting…
          </div>
        </div>
      );
    }
    return <ConnectScreen />;
  }

  return (
    <div className="app">
      <Shortcuts onSettings={() => setSettings(true)} />
      {sidebar && <Sidebar onSettings={() => setSettings(true)} />}
      <main className="main">
        <Header />
        <UsageBar />
        <Thread />
        <Todos />
        <Requests />
        <Composer />
      </main>
      <SidePanels />
      {settings && <SettingsDialog onClose={() => setSettings(false)} />}
    </div>
  );
}
