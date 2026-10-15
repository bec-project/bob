import { useCallback, useEffect, useRef, useState } from "react";
import { IconRefresh } from "../../components/Icons";
import type { PluginContext } from "../types";
import { buildCard, type Card } from "./cards";
import type { McpClient } from "./mcpClient";
import { ReportCard } from "./ReportCard";
import { callResult, serverWithTool, useBecClient } from "./server";

const QUEUE_MS = 5_000;

const CONTROLS = [
  { action: "resume", label: "Resume", confirm: "Resume the scan queue?" },
  { action: "halt", label: "Halt", confirm: "Halt the running scan? It can be resumed." },
  {
    action: "abort",
    label: "Abort",
    confirm: "Abort the running scan? This hits whatever runs, whoever started it.",
  },
] as const;

/**
 * The scan queue (refreshed every few seconds) and recent scans. Queue controls appear only when
 * a configured server offers bec_queue_control (a DEV-mode server); every press asks first.
 */
export function QueuePanel({ context }: { context: PluginContext }) {
  const { client, url, source } = useBecClient(context);
  const [queue, setQueue] = useState<Card>();
  const [history, setHistory] = useState<Card>();
  const [error, setError] = useState<string>();
  const [control, setControl] = useState<McpClient>();
  const [notice, setNotice] = useState<string>();
  const busy = useRef(false);

  const loadQueue = useCallback(async () => {
    if (!client || busy.current) return;
    busy.current = true;
    try {
      setQueue(buildCard("bec_queue", await callResult(client, "bec_queue", { history: 3 })));
      setError(undefined);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      busy.current = false;
    }
  }, [client]);

  const loadHistory = useCallback(async () => {
    if (!client) return;
    try {
      setHistory(buildCard("bec_scan_history", await callResult(client, "bec_scan_history", { limit: 10 })));
    } catch (e) {
      setError((e as Error).message);
    }
  }, [client]);

  useEffect(() => {
    void loadQueue();
    void loadHistory();
    const timer = setInterval(() => {
      if (!document.hidden) void loadQueue();
    }, QUEUE_MS);
    return () => clearInterval(timer);
  }, [loadQueue, loadHistory]);

  useEffect(() => {
    let cancelled = false;
    void serverWithTool(context, "bec_queue_control").then((found) => {
      if (!cancelled) setControl(found);
    });
    return () => {
      cancelled = true;
    };
  }, [context]);

  const run = async (action: string, question: string) => {
    if (!control || !window.confirm(question)) return;
    try {
      const result = await control.callTool<Record<string, unknown>>("bec_queue_control", { action });
      const err = result && typeof result === "object" ? result.error : undefined;
      setNotice(err ? `The queue did not take “${action}”: ${String(err)}` : `Sent “${action}” to the queue.`);
    } catch (e) {
      setNotice((e as Error).message);
    }
    void loadQueue();
  };

  if (!url) {
    return <div className="panel-message">No BEC MCP server found. Add one to bob.toml as <code>[mcp.bec_mcp]</code>.</div>;
  }

  return (
    <>
      <div className="panel-toolbar" title={`${url} (from ${source})`}>
        {control ? (
          CONTROLS.map((c) => (
            <button key={c.action} className={`btn small ${c.action === "abort" ? "danger" : ""}`} onClick={() => void run(c.action, c.confirm)}>
              {c.label}
            </button>
          ))
        ) : (
          <span className="muted" style={{ fontSize: 12 }}>Read-only: no server offers queue control.</span>
        )}
        <span className="grow" />
        <button
          className="icon-btn"
          title="Refresh"
          onClick={() => {
            void loadQueue();
            void loadHistory();
          }}
        >
          <IconRefresh size={14} />
        </button>
      </div>
      {notice && <div className="panel-message">{notice}</div>}
      {error && <div className="panel-message error">{error}</div>}
      {queue && (
        <div className="panel-section">
          <h4>Queue</h4>
          <div className="card-summary" style={{ marginBottom: 8, fontSize: 13 }}>{queue.summary}</div>
          <ReportCard card={queue} compact actions={{ ask: (text) => context.insertPrompt(text) }} />
        </div>
      )}
      {history && (
        <div className="panel-section" style={{ borderBottom: "none" }}>
          <h4>Recent scans</h4>
          {history.table?.rows.length ? <ReportCard card={history} compact /> : <div className="muted">{history.summary}</div>}
        </div>
      )}
    </>
  );
}
