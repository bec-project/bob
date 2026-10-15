import { useCallback, useEffect, useRef, useState } from "react";
import { IconRefresh, IconX } from "../../components/Icons";
import type { PluginContext } from "../types";
import { buildCard, type Card } from "./cards";
import { TracebackView, LogRecords, type LogActions } from "./LogViews";
import { ReportCard } from "./ReportCard";
import { callResult, serverTools, useBecClient } from "./server";

const WINDOWS = [
  { label: "15 min", minutes: 15 },
  { label: "1 h", minutes: 60 },
  { label: "6 h", minutes: 360 },
  { label: "All cached", minutes: 0 },
];
const REFRESH_MS = 30_000;

interface Detail {
  title: string;
  busy: boolean;
  error?: string;
  card?: Card;
}

/**
 * Problems in BEC's log, grouped into incidents by the server's bec_log_analysis. Falls back to
 * bec_logs on servers without the analyser. Code buttons appear when the server runs in code mode.
 */
export function IncidentsPanel({ context }: { context: PluginContext }) {
  const { client, url, source } = useBecClient(context);
  const [minutes, setMinutes] = useState(60);
  const [level, setLevel] = useState<"error" | "warning">("error");
  const [tools, setTools] = useState<Set<string>>();
  const [card, setCard] = useState<Card>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<Detail>();
  const busy = useRef(false);
  const detailRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (detail) detailRef.current?.scrollIntoView({ block: "start", behavior: "smooth" });
  }, [detail?.title]);

  const load = useCallback(async () => {
    if (!client || busy.current) return;
    busy.current = true;
    setLoading(true);
    try {
      const available = await serverTools(client);
      setTools(available);
      if (available.has("bec_log_analysis")) {
        const args: Record<string, unknown> = { level, limit: 20 };
        if (minutes) args.minutes = minutes;
        setCard(buildCard("bec_log_analysis", await callResult(client, "bec_log_analysis", args)));
      } else {
        setCard(buildCard("bec_logs", await callResult(client, "bec_logs", { level })));
      }
      setError(undefined);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      busy.current = false;
      setLoading(false);
    }
  }, [client, minutes, level]);

  useEffect(() => {
    void load();
    const timer = setInterval(() => {
      if (!document.hidden) void load();
    }, REFRESH_MS);
    return () => clearInterval(timer);
  }, [load]);

  const show = useCallback(
    async (title: string, tool: string, args: Record<string, unknown>) => {
      if (!client) return;
      setDetail({ title, busy: true });
      try {
        setDetail({ title, busy: false, card: buildCard(tool, await callResult(client, tool, args)) });
      } catch (e) {
        setDetail({ title, busy: false, error: (e as Error).message });
      }
    },
    [client],
  );

  const actions: LogActions = {
    ask: (text) => context.insertPrompt(text),
    showRecords: (record) => void show(`Records around #${record}`, "bec_log_range", { around: record, before: 8, after: 8 }),
    showCode: tools?.has("bec_code_traceback")
      ? (record) => void show(`Code behind record #${record}`, "bec_code_traceback", { record })
      : undefined,
  };

  if (!url) {
    return <div className="panel-message">No BEC MCP server found. Add one to bob.toml as <code>[mcp.bec_mcp]</code>.</div>;
  }

  return (
    <>
      <div className="panel-toolbar" title={`${url} (from ${source})`}>
        <select className="select" value={minutes} onChange={(event) => setMinutes(Number(event.target.value))}>
          {WINDOWS.map((w) => (
            <option key={w.minutes} value={w.minutes}>
              {w.label}
            </option>
          ))}
        </select>
        <select className="select" value={level} onChange={(event) => setLevel(event.target.value as "error" | "warning")}>
          <option value="error">Errors</option>
          <option value="warning">Warnings too</option>
        </select>
        <span className="grow" />
        {loading && <span className="spinner" />}
        <button className="icon-btn" title="Analyse again" onClick={() => void load()} disabled={loading}>
          <IconRefresh size={14} />
        </button>
      </div>
      {tools && !tools.has("bec_code_traceback") && tools.has("bec_log_analysis") && (
        <div className="panel-message" style={{ paddingTop: 8, paddingBottom: 0 }}>
          Start the MCP server with <code>--mode code</code> to see the code behind tracebacks.
        </div>
      )}
      {detail && (
        <div className="panel-section" ref={detailRef}>
          <h4>
            <span className="grow">{detail.title}</span>
            <button className="icon-btn" title="Close" onClick={() => setDetail(undefined)}>
              <IconX size={13} />
            </button>
          </h4>
          {detail.busy && <div className="working"><span className="spinner" /> Loading…</div>}
          {detail.error && <div className="panel-message error">{detail.error}</div>}
          {detail.card?.frames ? (
            <>
              <div className="card-summary" style={{ marginBottom: 8, fontSize: 12.5 }}>{detail.card.summary}</div>
              <TracebackView frames={detail.card.frames} exceptions={detail.card.exceptions ?? []} />
            </>
          ) : detail.card?.records ? (
            <LogRecords records={detail.card.records} actions={actions} />
          ) : detail.card ? (
            <ReportCard card={detail.card} compact actions={actions} />
          ) : null}
        </div>
      )}
      {error && <div className="panel-message error">{error}</div>}
      {card && (
        <div className="panel-section" style={{ borderBottom: "none" }}>
          <div className="card-summary" style={{ marginBottom: 10, fontSize: 13 }}>{card.summary}</div>
          <ReportCard card={{ ...card, findings: [] }} compact actions={actions} />
        </div>
      )}
    </>
  );
}
