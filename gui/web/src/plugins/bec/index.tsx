import { useState } from "react";
import { IconAlert, IconChip, IconQueue } from "../../components/Icons";
import type { ToolPart } from "../../api/types";
import type { GuiPlugin, PluginContext, ToolView } from "../types";
import { ACTIVITY, becToolName, buildCard, parseToolResult, scanResultCard, TITLES } from "./cards";
import { DevicePanel } from "./DevicePanel";
import { IncidentsPanel } from "./IncidentsPanel";
import type { LogActions } from "./LogViews";
import { QueuePanel } from "./QueuePanel";
import { ReportCard } from "./ReportCard";
import { EDITED_MARKER, ScanApproval } from "./ScanApproval";
import { formatScanCall } from "./scanCall";
import { becServerUrl, readToken, writeToken } from "./server";

/** In the chat, card buttons put a follow-up question into the composer. */
function chatActions(context: PluginContext): LogActions {
  return {
    ask: (text) => context.insertPrompt(text),
    showCode: (record) => context.insertPrompt(`Show the code behind the traceback in BEC log record ${record}.`),
    showRecords: (record) => context.insertPrompt(`Show the BEC log records around record ${record}.`),
  };
}

function describeTool(part: ToolPart, context: PluginContext): ToolView | undefined {
  const name = becToolName(part.tool);
  const state = part.state;
  if (state.status === "pending" || state.status === "running") {
    return name ? { label: ACTIVITY[name] ?? TITLES[name] } : undefined;
  }
  if (state.status === "error") {
    if (name === "bec_run_scan" && state.error.includes(EDITED_MARKER)) {
      const detail = state.error.slice(state.error.indexOf(EDITED_MARKER) + EDITED_MARKER.length).replace(/^:\s*/, "");
      return { label: "Scan (edited by you)", summary: detail, tone: "ok" };
    }
    return name ? { label: TITLES[name] } : undefined;
  }

  const result = parseToolResult(state.output);
  if (name === "bec_run_scan") {
    const callText = formatScanCall(state.input);
    const raw = result?.data ?? (() => {
      try {
        return JSON.parse(state.output) as Record<string, unknown>;
      } catch {
        return undefined;
      }
    })();
    if (!raw) return { label: "Scan", summary: callText };
    const card = scanResultCard(raw, callText);
    return { label: "Scan", summary: card.summary, tone: card.tone, body: <ReportCard card={card} compact /> };
  }
  if (!result) return name ? { label: TITLES[name] } : undefined;
  // Unknown tools from a BEC server still share the result shape: draw summary and findings.
  if (!name && !/bec|mcp_status/i.test(part.tool)) return undefined;
  const kind = name ?? part.tool;
  const card = buildCard(kind, result);
  return {
    label: TITLES[kind] ?? undefined,
    summary: result.summary,
    tone: card.tone,
    body: <ReportCard card={card} compact actions={chatActions(context)} />,
  };
}

function Settings({ context }: { context: PluginContext }) {
  const [token, setToken] = useState(readToken);
  const auto = becServerUrl({ ...context, settings: {} });
  return (
    <div style={{ display: "grid", gap: 8, marginTop: 8 }}>
      <label className="desc">
        MCP server URL for the BEC panels
        <input
          className="input"
          placeholder={auto.url ? `${auto.url} (from ${auto.source})` : "http://127.0.0.1:8000/mcp"}
          value={String(context.settings.mcpUrl ?? "")}
          onChange={(event) => context.setSettings({ mcpUrl: event.target.value })}
        />
      </label>
      <label className="desc">
        Bearer token, if the server needs one (kept for this window only)
        <input
          className="input"
          type="password"
          value={token}
          onChange={(event) => {
            setToken(event.target.value);
            writeToken(event.target.value);
          }}
        />
      </label>
    </div>
  );
}

/**
 * BEC: report cards for the BEC MCP server's tool results, editable scan approvals, and panels
 * for devices, log incidents and the scan queue that read the server directly.
 */
export const becPlugin: GuiPlugin = {
  id: "bec",
  name: "BEC",
  description:
    "Report cards for BEC MCP results (logs, incidents and tracebacks included), editable scan approvals, and panels for devices, incidents and the queue.",
  defaultEnabled: false,
  describeTool,
  describePermission(_request, tool, context) {
    if (!tool || becToolName(tool.tool) !== "bec_run_scan") return undefined;
    return {
      title: "Run this scan?",
      body: (controls) => <ScanApproval tool={tool} context={context} controls={controls} />,
    };
  },
  panels: [
    {
      id: "devices",
      title: "BEC devices",
      icon: <IconChip size={18} />,
      render: (context) => <DevicePanel context={context} />,
    },
    {
      id: "incidents",
      title: "BEC log incidents",
      icon: <IconAlert size={18} />,
      width: 460,
      render: (context) => <IncidentsPanel context={context} />,
    },
    {
      id: "queue",
      title: "BEC queue and scans",
      icon: <IconQueue size={18} />,
      width: 420,
      render: (context) => <QueuePanel context={context} />,
    },
  ],
  suggestions: [
    "Is the beamline healthy?",
    "What went wrong in the BEC logs in the last hour?",
    "Show the last five scans",
    "What arguments does a line scan take?",
  ],
  settingsView: (context) => <Settings context={context} />,
};
