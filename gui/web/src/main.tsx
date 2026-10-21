import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { inTauri } from "./platform";
import "./styles.css";

/** Shown when the page is opened in a browser: only the desktop app can start and reach bob. */
function DesktopOnly() {
  return (
    <div className="connect">
      <div className="connect-card">
        <h1>bob</h1>
        <p>This page runs inside bob's desktop app.</p>
        <div className="connect-hint">
          Start it from the repository root:
          <pre style={{ marginTop: 8 }}>
            <code>npm --prefix gui/web run bob</code>
          </pre>
        </div>
      </div>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>{inTauri() ? <App /> : <DesktopOnly />}</StrictMode>,
);
