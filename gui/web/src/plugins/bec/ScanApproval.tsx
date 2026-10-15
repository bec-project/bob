import { useMemo, useState } from "react";
import type { ToolPart } from "../../api/types";
import type { PermissionControls, PluginContext } from "../types";
import { parseToolResult, scanResultCard } from "./cards";
import { formatScanCall, parseScanCall, ScanCallError, scanToolArgs } from "./scanCall";
import { mcpClient, readToken, serverForToolId } from "./server";

/** What the rejection tells the model, and what the tool row recognises afterwards. */
export const EDITED_MARKER = "edited the scan and ran it from the GUI instead";

/**
 * The approval card for bec_run_scan: the call as the user would type it, editable. Allow runs
 * the agent's call unchanged; Run edited runs the edited call from the GUI on the same server,
 * then rejects the agent's call with a message saying what ran, so the agent knows.
 */
export function ScanApproval({
  tool,
  context,
  controls,
}: {
  tool: ToolPart;
  context: PluginContext;
  controls: PermissionControls;
}) {
  const original = useMemo(() => formatScanCall(tool.state.input), [tool.state.input]);
  const [text, setText] = useState(original);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const edited = text.trim() !== original;
  const parsed = useMemo(() => {
    try {
      return { call: parseScanCall(text) };
    } catch (e) {
      return { error: e instanceof ScanCallError ? e.message : String(e) };
    }
  }, [text]);
  const server = serverForToolId(context, tool.tool);

  const runEdited = async () => {
    if (!parsed.call || !server) return;
    setBusy(true);
    setError(undefined);
    try {
      // keep the agent's other arguments (wait, timeout) and replace only the scan itself
      const input: Record<string, unknown> = { ...tool.state.input, ...scanToolArgs(parsed.call) };
      delete input.scan_name;
      const raw = await mcpClient(server.url, readToken()).callTool<Record<string, unknown>>("bec_run_scan", input);
      const result = (parseToolResult(raw)?.data ?? raw) as Record<string, unknown>;
      const summary = scanResultCard(result, text.trim()).summary;
      await controls.reply("reject", `The user ${EDITED_MARKER}: ${text.trim()}. Result: ${summary}`);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="scan-approval">
      <textarea
        className="input"
        rows={Math.min(6, Math.max(2, Math.ceil(text.length / 70)))}
        spellCheck={false}
        value={text}
        onChange={(event) => setText(event.target.value)}
        style={{ fontFamily: "var(--mono)", fontSize: 12.5 }}
      />
      {edited && parsed.error && <div className="tone-error" style={{ fontSize: 12.5 }}>{parsed.error}</div>}
      {edited && !parsed.error && (
        <div className="row">
          <button className="btn primary" disabled={busy || !server} onClick={() => void runEdited()}>
            {busy ? "Running…" : "Run edited scan"}
          </button>
          <button className="btn ghost" disabled={busy} onClick={() => setText(original)}>
            Undo edit
          </button>
          <span className="muted" style={{ fontSize: 12.5 }}>Allow once still runs the agent's original call.</span>
          {!server && <span className="muted" style={{ fontSize: 12.5 }}>No configured server matches {tool.tool}.</span>}
        </div>
      )}
      {!edited && (
        <div className="muted" style={{ fontSize: 12.5 }}>
          Edit the call to change it; Allow once runs it as written.
        </div>
      )}
      {error && <div className="tone-error" style={{ fontSize: 12.5 }}>{error}</div>}
    </div>
  );
}
