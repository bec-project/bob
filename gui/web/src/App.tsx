import { useEffect, useState } from "react";
import { bobStatus, onBobStatus, type BobStatus } from "./platform";
import { Composer } from "./components/Composer";
import { ConnectScreen } from "./components/ConnectScreen";
import { Header, UsageBar } from "./components/Header";
import { IconAlert } from "./components/Icons";
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

/** Follows the app's lease on bob, which Rust holds and reports. */
function useBobStatus() {
  const setBob = useStore((s) => s.setBob);
  useEffect(() => {
    let stop: (() => void) | undefined;
    let cancelled = false;
    void onBobStatus(setBob).then((unlisten) => {
      if (cancelled) unlisten();
      else stop = unlisten;
    });
    return () => {
      cancelled = true;
      stop?.();
    };
  }, [setBob]);
}

/**
 * Connect on start to the bob that Rust holds or is joining: it joins by itself when the app
 * opens (unless that is switched off), and keeps its lease when the page reloads.
 */
function useAutoConnect() {
  const [checked, setChecked] = useState(false);
  const joinBob = useStore((s) => s.joinBob);
  const setBob = useStore((s) => s.setBob);
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const status = await bobStatus().catch((): BobStatus => ({ state: "idle", error: null }));
      if (cancelled) return;
      setBob(status);
      if (status.state === "joined" || status.state === "joining") {
        // returns the lease the app holds, or waits for the join in progress
        await joinBob();
      } else if (status.state === "lost") {
        useStore.setState({ connectError: status.reason });
      } else if (status.error) {
        useStore.setState({ connectError: status.error });
      }
      setChecked(true);
    })();
    return () => {
      cancelled = true;
    };
  }, [joinBob, setBob]);
  return checked;
}

/** Shown over the chat when the broker ended the lease on the bob this window uses. */
function BobLost() {
  const bob = useStore((s) => s.bob);
  const connection = useStore((s) => s.connection);
  const joinBob = useStore((s) => s.joinBob);
  const leaveBob = useStore((s) => s.leaveBob);
  if (bob.state !== "lost" || bob.container !== connection?.container) return null;
  return (
    <div className="banner warn" role="alert">
      <IconAlert size={14} />
      <span className="grow">{bob.reason}</span>
      <button className="btn small" onClick={() => void joinBob()}>
        Join again
      </button>
      <button className="btn small ghost" onClick={() => void leaveBob()}>
        Switch bob…
      </button>
    </div>
  );
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
  useBobStatus();
  const checked = useAutoConnect();
  const phase = useStore((s) => s.phase);
  const sidebar = useStore((s) => s.prefs.sidebar);
  const bob = useStore((s) => s.bob);
  const leaveBob = useStore((s) => s.leaveBob);
  const [settings, setSettings] = useState(false);

  if (phase !== "ready") {
    if (phase === "starting") {
      return (
        <div className="connect">
          <div className="working">
            <span className="spinner" /> Joining your bob
            {bob.state === "joining" ? `, or starting it with ${bob.config.split("/").pop()}` : ""}…
            <button className="btn small ghost" onClick={() => void leaveBob()}>
              Cancel
            </button>
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
        <BobLost />
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
