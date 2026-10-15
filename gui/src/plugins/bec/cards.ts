// BEC tool results turned into report cards: a TypeScript port of bec_mcp_chat's cards.py.
// Every card has the same shape, so one component draws all of them.

import type { Tone } from "../types";

export interface Cell {
  text: string;
  tone?: Tone;
}

export interface Finding {
  severity: "error" | "warning" | "info";
  tone: Tone;
  text: string;
  next: string;
}

export interface LogLine {
  id?: number;
  time: string;
  service: string;
  level: string;
  tone: Tone;
  message: string;
  /** "module:function:line" where the record was logged. */
  source?: string;
  /** The exception a traceback in the record ends in, and where it was raised. */
  traceback?: { exception: string; origin: string };
}

export interface IncidentView {
  ref: string;
  title: string;
  kind: string;
  tone: Tone;
  count: number;
  first: string;
  last: string;
  services: string[];
  devices: string[];
  scan: string;
  related: string[];
  message: string;
  likelyCause: string;
  suggestion: string;
  /** Where the traceback was raised: path:line (function), and that line. */
  origin?: { where: string; code: string };
  rootCause: string;
  firstId?: number;
  lastId?: number;
  /** A record of this incident that carries a traceback, for bec_code_traceback(record=…). */
  tracebackRecord?: number;
  context: LogLine[];
}

export interface FrameView {
  index: number;
  where: string;
  function: string;
  /** Numbered source lines; the raising line starts with ">". */
  code: string;
  underRoots: boolean;
  /** "matches", or why the file may not be the code that ran. */
  sourceCheck: string;
  exceptionIndex: number;
}

export interface Card {
  kind: string;
  title: string;
  summary: string;
  tone: Tone;
  badges: { text: string; tone: Tone }[];
  stats: { label: string; value: string; tone?: Tone }[];
  gauge?: {
    label: string;
    value: string;
    age: string;
    low: string;
    high: string;
    /** Position between the limits, 0 to 1, or -1 without limits. */
    fraction: number;
    tone: Tone;
  };
  table?: { columns: string[]; rows: Cell[][]; note: string };
  findings: Finding[];
  code?: { label: string; text: string };
  links: { title: string; subtitle: string; url: string; snippet: string }[];
  lines: { text: string; tone: Tone; meta: string }[];
  incidents?: IncidentView[];
  records?: LogLine[];
  frames?: FrameView[];
  exceptions?: { text: string; relation: string }[];
}

/** The common result shape of every BEC MCP tool (tool_result() on the server). */
export interface ToolResult {
  ok?: boolean;
  summary: string;
  findings?: string[];
  data?: Record<string, unknown>;
  truncated?: boolean;
}

type Data = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

const TONE_FOR_STATUS: Record<string, Tone> = {
  RUNNING: "ok",
  COMPLETED: "ok",
  closed: "ok",
  IDLE: "ok",
  PENDING: "info",
  PAUSED: "warn",
  LOCKED: "warn",
  HALTED: "warn",
  user_completed: "info",
  STOPPED: "error",
  ERROR: "error",
  aborted: "error",
  halted: "warn",
  BUSY: "info",
};
const SEVERITY_TONE: Record<string, Tone> = { error: "error", warning: "warn", info: "info" };
const LEVEL_TONE: Record<string, Tone> = {
  ERROR: "error",
  CRITICAL: "error",
  WARNING: "warn",
  SUCCESS: "ok",
  INFO: "neutral",
  DEBUG: "neutral",
};

export const TITLES: Record<string, string> = {
  bec_status: "BEC services",
  bec_ping: "Connection",
  bec_devices: "Devices",
  bec_device_info: "Device",
  bec_device_read: "Reading",
  bec_queue: "Scan queue",
  bec_beamline_states: "Beamline states",
  bec_scans: "Available scans",
  bec_scan_help: "Scan",
  bec_scan_history: "Recent scans",
  bec_logs: "Service logs",
  bec_diagnose: "Beamline diagnosis",
  bec_device_diagnose: "Device diagnosis",
  bec_docs_search: "Documentation",
  bec_docs_page: "Documentation",
  mcp_status: "MCP server",
  bec_run_scan: "Scan",
  bec_log_range: "Log records",
  bec_log_analysis: "Log analysis",
  bec_code_traceback: "Traceback",
  bec_code_read: "Code",
  bec_code_search: "Code search",
  bec_queue_control: "Queue control",
  bec_service_control: "Service control",
};

/** What the tool row says while the call runs, in the user's words. */
export const ACTIVITY: Record<string, string> = {
  bec_status: "Checking BEC services",
  bec_ping: "Pinging BEC",
  bec_devices: "Listing devices",
  bec_device_info: "Looking up the device",
  bec_device_read: "Reading the device",
  bec_queue: "Checking the scan queue",
  bec_beamline_states: "Checking beamline states",
  bec_scans: "Listing scans",
  bec_scan_help: "Looking up the scan",
  bec_scan_history: "Reading the scan history",
  bec_logs: "Reading service logs",
  bec_diagnose: "Diagnosing the beamline",
  bec_device_diagnose: "Diagnosing the device",
  bec_docs_search: "Searching the docs",
  bec_docs_page: "Reading the docs",
  mcp_status: "Checking the MCP server",
  bec_run_scan: "Running the scan",
  bec_log_range: "Reading the log",
  bec_log_analysis: "Analysing the log",
  bec_code_traceback: "Reading the code behind the traceback",
  bec_code_read: "Reading code",
  bec_code_search: "Searching the code",
  bec_queue_control: "Controlling the queue",
  bec_service_control: "Controlling the services",
};

export const toneFor = (status: unknown): Tone => TONE_FOR_STATUS[String(status)] ?? "neutral";

export function text(value: unknown): string {
  if (value === null || value === undefined) return "–";
  if (typeof value === "boolean") return value ? "yes" : "no";
  if (typeof value === "number") {
    return Number.isInteger(value) ? String(value) : String(Number(value.toPrecision(6)));
  }
  if (Array.isArray(value)) return value.map(text).join(", ") || "–";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

const cell = (value: unknown, tone?: Tone): Cell => ({ text: text(value), tone });

export function duration(seconds: unknown): string {
  const s = Number(seconds);
  if (seconds === null || seconds === undefined || Number.isNaN(s)) return "–";
  if (s < 1) return `${(s * 1000).toFixed(0)} ms`;
  if (s < 90) return `${s.toFixed(1)} s`;
  if (s < 5400) return `${(s / 60).toFixed(0)} min`;
  if (s < 172800) return `${(s / 3600).toFixed(1)} h`;
  return `${(s / 86400).toFixed(1)} d`;
}

export function emptyCard(kind: string, title: string, summary = "", tone: Tone = "neutral"): Card {
  return { kind, title, summary, tone, badges: [], stats: [], findings: [], links: [], lines: [] };
}

function findings(answer: ToolResult): Finding[] {
  const texts = answer.findings ?? [];
  const structured = ((answer.data ?? {}) as Data).findings ?? [];
  return texts.map((raw, index) => {
    const info = (structured[index] ?? {}) as Data;
    let severity = String(info.severity ?? String(raw).split(":", 1)[0].trim().toLowerCase());
    if (!(severity in SEVERITY_TONE)) severity = "info";
    const value = String(raw);
    const split = value.indexOf(" Next: ");
    let body = split === -1 ? value : value.slice(0, split);
    const next = split === -1 ? "" : value.slice(split + 7).replace(/\.$/, "").trim();
    const prefix = `${severity[0].toUpperCase()}${severity.slice(1)}: `;
    if (body.startsWith(prefix)) body = body.slice(prefix.length);
    return {
      severity: severity as Finding["severity"],
      tone: SEVERITY_TONE[severity],
      text: body.trim(),
      next,
    };
  });
}

function overallTone(list: Finding[], ok: boolean): Tone {
  if (!ok) return "error";
  const severities = new Set(list.map((f) => f.severity));
  if (severities.has("error")) return "error";
  if (severities.has("warning")) return "warn";
  return "ok";
}

function statusCard(card: Card, data: Data) {
  const redis = (data.redis ?? {}) as Data;
  if (Object.keys(redis).length) {
    const rtt = redis.round_trip_ms;
    card.badges.push({
      text: `Redis ${redis.address ?? ""}${typeof rtt === "number" ? ` · ${rtt.toFixed(1)} ms` : ""}`,
      tone: redis.answered ? "ok" : "error",
    });
  }
  const services = [...(data.core_services ?? []), ...(data.other_services ?? [])] as Data[];
  const rows = services.map((s) => [
    cell(s.name),
    cell(s.status, toneFor(s.status)),
    cell(`${Number(s.cpu_percent ?? 0).toFixed(1)} %`),
    cell(`${Number(s.memory_mb ?? 0).toFixed(0)} MB`),
    cell(duration(s.uptime_s)),
    cell(s.bec_lib),
  ]);
  if (rows.length) {
    card.table = { columns: ["Service", "Status", "CPU", "Memory", "Uptime", "bec_lib"], rows, note: "" };
  }
  const clients = (data.clients ?? {}) as Data;
  if (clients.count !== undefined && clients.count !== null) {
    card.stats.push({ label: "Clients", value: String(clients.count) });
  }
}

function pingCard(card: Card, data: Data) {
  if (typeof data.round_trip_ms === "number") {
    card.stats.push({ label: "round trip", value: `${data.round_trip_ms.toFixed(1)} ms` });
  }
  if (data.services !== undefined) card.stats.push({ label: "services", value: text(data.services) });
}

function devicesCard(card: Card, data: Data) {
  for (const [kind, count] of Object.entries((data.by_type ?? {}) as Data)) {
    card.stats.push({ label: kind.replace(/_/g, " "), value: String(count) });
  }
  const rows = ((data.devices ?? []) as Data[]).map((d) => [
    cell(d.name, "accent"),
    cell(d.type),
    cell(d.device_class),
    cell(d.readout_priority),
    cell(d.enabled ? "enabled" : "disabled", d.enabled ? "ok" : "warn"),
  ]);
  let note = "";
  const { total, returned } = data;
  if (typeof total === "number" && typeof returned === "number" && total > returned) {
    const start = (data.offset ?? 0) + 1;
    note = `Showing ${start}–${start + returned - 1} of ${total}`;
  }
  card.table = { columns: ["Name", "Type", "Class", "Readout", "State"], rows, note };
}

function deviceReadCard(card: Card, data: Data) {
  const name = String(data.name ?? "");
  const egu = String(data.egu ?? "");
  const readback = (data.readback ?? {}) as Record<string, Data>;
  const main = readback[name] ?? Object.values(readback)[0] ?? {};
  const value = main.value;
  const limits = (data.limits ?? {}) as Data;
  const { low, high } = limits;
  if (typeof value === "number") {
    const gauge: NonNullable<Card["gauge"]> = {
      label: name,
      value: `${text(value)} ${egu}`.trim(),
      age: main.age_s ? `updated ${duration(main.age_s)} ago` : "",
      low: "",
      high: "",
      fraction: -1,
      tone: "ok",
    };
    if (typeof low === "number" && typeof high === "number" && high > low) {
      const fraction = (value - low) / (high - low);
      gauge.low = text(low);
      gauge.high = text(high);
      gauge.fraction = Math.max(0, Math.min(1, fraction));
      if (fraction <= 0.02 || fraction >= 0.98) gauge.tone = "warn";
    }
    card.gauge = gauge;
  }
  const signals = [
    ...Object.entries(readback),
    ...Object.entries((data.configuration ?? {}) as Record<string, Data>),
  ];
  const rows = signals.map(([signal, info]) => [
    cell(signal),
    cell(info?.value, signal === name ? "accent" : undefined),
    cell(duration(info?.age_s)),
  ]);
  if (rows.length) card.table = { columns: ["Signal", "Value", "Age"], rows, note: "" };
  const enabled = data.enabled ?? true;
  card.badges.push({ text: enabled ? "enabled" : "disabled", tone: enabled ? "ok" : "warn" });
  if (data.type) card.badges.push({ text: String(data.type), tone: "neutral" });
}

function deviceInfoCard(card: Card, data: Data) {
  const rows: Cell[][] = [];
  for (const key of ["name", "type", "device_class", "readout_priority", "enabled", "description"]) {
    if (data[key] !== undefined) rows.push([cell(key.replace(/_/g, " ")), cell(data[key])]);
  }
  if (rows.length) card.table = { columns: ["Field", "Value"], rows, note: "" };
}

function queueCard(card: Card, data: Data) {
  for (const [name, queue] of Object.entries((data.queues ?? {}) as Record<string, Data>)) {
    card.badges.push({ text: `${name}: ${queue.status}`, tone: toneFor(queue.status) });
    card.stats.push({ label: `${name} pending`, value: String(queue.size ?? 0) });
    for (const lock of (queue.locks ?? []) as unknown[]) {
      card.findings.push({ severity: "warning", tone: "warn", text: `Locked: ${text(lock)}`, next: "" });
    }
  }
  const rows: Cell[][] = [];
  const running = data.running as Data | undefined;
  if (running) {
    rows.push([
      cell(text(running.scan_numbers ?? running.scan_number)),
      cell(text(running.requests ?? running.scan_name)),
      cell(running.status ?? "RUNNING", "info"),
      cell("now"),
    ]);
  }
  for (const item of (data.history ?? []) as Data[]) {
    rows.push([
      cell(text(item.scan_numbers)),
      cell(text(item.requests)),
      cell(item.status, toneFor(item.status)),
      cell(item.reason ?? ""),
    ]);
  }
  if (rows.length) card.table = { columns: ["Scan #", "Request", "Status", "Note"], rows, note: "" };
}

function historyCard(card: Card, data: Data) {
  const rows = ((data.scans ?? []) as Data[]).map((scan) => [
    cell(scan.scan_number, "accent"),
    cell(scan.scan_name),
    cell(scan.status, toneFor(scan.status)),
    cell(duration(scan.duration_s)),
    cell(scan.num_points),
    cell(String(scan.start_time ?? "").slice(0, 19).replace("T", " ")),
  ]);
  card.table = { columns: ["#", "Scan", "Status", "Duration", "Points", "Started"], rows, note: "" };
}

function scansCard(card: Card, data: Data) {
  const rows = ((data.scans ?? []) as Data[]).map((scan) => [
    cell(scan.name, "accent"),
    cell(scan.description),
  ]);
  card.table = { columns: ["Scan", "Description"], rows, note: "" };
}

function scanHelpCard(card: Card, data: Data) {
  card.title = `Scan · ${data.name ?? ""}`;
  const rows = ((data.arguments ?? []) as Data[]).map((arg) => {
    const extra: string[] = [];
    if ("default" in arg) extra.push(`default ${text(arg.default)}`);
    if (arg.units) extra.push(String(arg.units));
    if (arg.constraint) extra.push(String(arg.constraint));
    const detail = String(arg.description ?? "").replace(/\.$/, "");
    return [
      cell(arg.name, arg.required ? "accent" : undefined),
      cell(arg.type),
      cell(arg.required ? "required" : "optional", arg.required ? "info" : undefined),
      cell([detail, extra.join(", ")].filter(Boolean).join(" · ")),
    ];
  });
  card.table = { columns: ["Argument", "Type", "", "Details"], rows, note: "" };
  if (data.example) card.code = { label: "Example call", text: String(data.example) };
  if (data.repeat) card.badges.push({ text: String(data.repeat), tone: "neutral" });
}

function logsCard(card: Card, data: Data) {
  for (const [level, count] of Object.entries((data.levels ?? {}) as Data)) {
    card.stats.push({ label: level.toLowerCase(), value: String(count), tone: LEVEL_TONE[level] });
  }
  for (const entry of ((data.entries ?? []) as Data[]).slice(0, 40)) {
    const repeat = (entry.count ?? 1) > 1 ? ` ×${entry.count}` : "";
    card.lines.push({
      text: String(entry.message ?? "").slice(0, 400),
      tone: LEVEL_TONE[String(entry.level)] ?? "neutral",
      meta: `${entry.service ?? ""} · ${entry.level ?? ""}${repeat} · ${String(entry.last ?? "").slice(11, 19)}`,
    });
  }
  for (const alarm of (data.alarms ?? []) as unknown[]) {
    card.findings.push({ severity: "error", tone: "error", text: text(alarm).slice(0, 400), next: "" });
  }
}

function diagnoseCard(card: Card, data: Data) {
  const services = data.services;
  if (services && typeof services === "object" && !Array.isArray(services)) {
    const values = Object.values(services as Data);
    if (values.length) {
      const up = values.filter((status) => toneFor(status) === "ok").length;
      card.stats.push({
        label: "services up",
        value: `${up}/${values.length}`,
        tone: up === values.length ? "ok" : "warn",
      });
    }
  }
  const devices = (data.devices ?? {}) as Data;
  if ("configured" in devices) {
    card.stats.push({ label: "devices", value: String(devices.configured) });
    if (devices.disabled) card.stats.push({ label: "disabled", value: String(devices.disabled), tone: "warn" });
  }
  for (const [name, queue] of Object.entries((data.queues ?? {}) as Record<string, Data>)) {
    card.stats.push({ label: `${name} queue`, value: String(queue.status ?? ""), tone: toneFor(queue.status) });
  }
  if (data.readback !== undefined && data.readback !== null) {
    const [low, high] = (data.limits ?? []) as number[];
    deviceReadCard(card, {
      name: data.device,
      readback: { [String(data.device)]: { value: data.readback, age_s: data.readback_age_s } },
      limits: { low, high },
      enabled: data.enabled ?? true,
    });
    card.table = undefined;
  }
}

function docsCard(card: Card, data: Data) {
  for (const result of (data.results ?? []) as Data[]) {
    card.links.push({
      title: String(result.title ?? result.path ?? ""),
      subtitle: String(result.heading ?? ""),
      url: String(result.url ?? ""),
      snippet: String(result.snippet ?? "").slice(0, 240),
    });
  }
}

function statesCard(card: Card, data: Data) {
  const interlock = (data.interlock ?? {}) as Data;
  if (Object.keys(interlock).length) {
    card.badges.push({
      text: interlock.enabled ? "interlock on" : "interlock off",
      tone: interlock.enabled ? "info" : "neutral",
    });
    if (interlock.primary_queue_locked) card.badges.push({ text: "queue locked", tone: "warn" });
  }
  const rows = ((data.states ?? []) as Data[]).map((state) => [
    cell(state.name),
    cell(state.status ?? state.state, toneFor(state.status)),
  ]);
  if (rows.length) card.table = { columns: ["State", "Status"], rows, note: "" };
}

const clock = (iso: unknown) => String(iso ?? "").slice(11, 19);

function logLine(record: Data): LogLine {
  const tb = record.traceback as Data | undefined;
  const origin = tb?.origin as Data | undefined;
  return {
    id: typeof record.id === "number" ? record.id : undefined,
    time: clock(record.time),
    service: String(record.service ?? ""),
    level: String(record.level ?? ""),
    tone: LEVEL_TONE[String(record.level ?? "").toUpperCase()] ?? "neutral",
    message: String(record.message ?? ""),
    source: record.source ? String(record.source) : undefined,
    traceback: tb
      ? {
          exception: String(tb.root_cause ?? tb.exception ?? ""),
          origin: origin ? `${origin.path ?? origin.file}:${origin.line} (${origin.function})` : "",
        }
      : undefined,
  };
}

function incident(raw: Data): IncidentView {
  const tb = raw.traceback as Data | undefined;
  const origin = tb?.origin as Data | undefined;
  const scan = raw.scan as Data | undefined;
  const ids = (raw.ids ?? []) as number[];
  return {
    ref: String(raw.ref ?? ""),
    title: String(raw.title ?? ""),
    kind: String(raw.kind ?? ""),
    tone: SEVERITY_TONE[String(raw.severity)] ?? (raw.severity === "critical" ? "error" : "neutral"),
    count: Number(raw.count ?? 1),
    first: String(raw.first ?? ""),
    last: String(raw.last ?? ""),
    services: (raw.services ?? []) as string[],
    devices: (raw.devices ?? []) as string[],
    scan: scan ? [scan.name, scan.scan_number !== undefined ? `#${scan.scan_number}` : ""].filter(Boolean).join(" ") : "",
    related: (raw.related ?? []) as string[],
    message: String(raw.message ?? ""),
    likelyCause: String(raw.likely_cause ?? ""),
    suggestion: String(raw.suggestion ?? ""),
    origin: origin
      ? { where: `${origin.path ?? origin.file}:${origin.line} (${origin.function})`, code: String(origin.code ?? "") }
      : undefined,
    rootCause: String(tb?.root_cause ?? ""),
    firstId: typeof raw.first_id === "number" ? raw.first_id : undefined,
    lastId: typeof raw.last_id === "number" ? raw.last_id : undefined,
    tracebackRecord: tb ? (ids[ids.length - 1] ?? raw.last_id) : undefined,
    context: [...((raw.before ?? []) as Data[]), ...((raw.after ?? []) as Data[])].map(logLine),
  };
}

function logAnalysisCard(card: Card, data: Data) {
  const cache = (data.cache ?? {}) as Data;
  if (data.incidents_found !== undefined) {
    card.stats.push({ label: "incidents", value: String(data.incidents_found), tone: data.incidents_found ? "warn" : "ok" });
  }
  if (cache.records !== undefined) {
    card.stats.push({ label: "records cached", value: `${cache.records}${cache.capacity ? ` / ${cache.capacity}` : ""}` });
  }
  card.incidents = ((data.incidents ?? []) as Data[]).map(incident);
}

function logRangeCard(card: Card, data: Data) {
  card.records = ((data.records ?? []) as Data[]).map(logLine);
  const selection = (data.selection ?? {}) as Data;
  if (selection.first_id !== undefined) {
    card.badges.push({ text: `records ${selection.first_id}–${selection.last_id}`, tone: "neutral" });
  }
}

function codeTracebackCard(card: Card, data: Data) {
  card.exceptions = ((data.exceptions ?? []) as Data[]).map((e) => ({
    text: String(e.exception ?? ""),
    relation: String(e.relation ?? ""),
  }));
  card.frames = ((data.frames ?? []) as Data[]).map((frame) => ({
    index: Number(frame.frame ?? 0),
    where: `${frame.file}:${frame.line}`,
    function: String(frame.in ?? frame.function ?? ""),
    code: String(frame.code ?? ""),
    underRoots: frame.under_roots !== false,
    sourceCheck: String(frame.source_check ?? ""),
    exceptionIndex: Number(frame.exception_index ?? 0),
  }));
  const record = data.record as Data | undefined;
  if (record?.service) card.badges.push({ text: `${record.service} · record ${record.id}`, tone: "neutral" });
}

function codeReadCard(card: Card, data: Data) {
  const range = (data.range ?? []) as number[];
  const symbol = (data.symbol ?? {}) as Data;
  const label = [data.file, symbol.name, range.length === 2 ? `lines ${range[0]}–${range[1]}` : ""].filter(Boolean).join(" · ");
  if (data.code) card.code = { label, text: String(data.code) };
}

function codeSearchCard(card: Card, data: Data) {
  const rows = ((data.matches ?? []) as Data[]).map((m) => [
    cell(`${m.file}:${m.line}`, "accent"),
    cell(m.in ?? ""),
    cell(m.text),
  ]);
  card.table = { columns: ["Where", "In", "Line"], rows, note: "" };
}

const BUILDERS: Record<string, (card: Card, data: Data) => void> = {
  bec_log_analysis: logAnalysisCard,
  bec_log_range: logRangeCard,
  bec_code_traceback: codeTracebackCard,
  bec_code_read: codeReadCard,
  bec_code_search: codeSearchCard,
  bec_status: statusCard,
  bec_ping: pingCard,
  bec_devices: devicesCard,
  bec_device_info: deviceInfoCard,
  bec_device_read: deviceReadCard,
  bec_queue: queueCard,
  bec_scan_history: historyCard,
  bec_scans: scansCard,
  bec_scan_help: scanHelpCard,
  bec_logs: logsCard,
  bec_diagnose: diagnoseCard,
  bec_device_diagnose: diagnoseCard,
  bec_docs_search: docsCard,
  bec_beamline_states: statesCard,
};

/** The BEC tools the cards know, longest first so suffix matching picks the most specific. */
export const KNOWN_TOOLS = Object.keys(TITLES).sort((a, b) => b.length - a.length);

/**
 * The BEC tool behind an OpenCode tool id. OpenCode prefixes MCP tools with the server's name
 * from bob.toml, e.g. bec_mcp_bec_devices for bec_devices on the server named bec_mcp.
 */
export function becToolName(toolID: string): string | undefined {
  return KNOWN_TOOLS.find((name) => toolID === name || toolID.endsWith(`_${name}`));
}

/** The tool_result() object in a tool's output text, or undefined if it is not one. */
export function parseToolResult(output: unknown): ToolResult | undefined {
  let value = output;
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed.startsWith("{")) return undefined;
    try {
      value = JSON.parse(trimmed);
    } catch {
      return undefined;
    }
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const result = value as ToolResult;
  return typeof result.summary === "string" ? result : undefined;
}

/** A card for one tool result. */
export function buildCard(tool: string, answer: ToolResult): Card {
  const list = findings(answer);
  const card = emptyCard(
    tool,
    TITLES[tool] ?? tool.replace(/_/g, " "),
    answer.summary ?? "",
    overallTone(list, answer.ok ?? true),
  );
  card.findings = list;
  const builder = BUILDERS[tool];
  const data = answer.data;
  if (builder && data && typeof data === "object") {
    try {
      builder(card, data as Data);
    } catch {
      // a shape this version does not know: the summary and findings still show
    }
  }
  if (answer.truncated) card.badges.push({ text: "truncated", tone: "warn" });
  return card;
}

/** The card after a scan submission through bec_run_scan. */
export function scanResultCard(result: Data, callText: string): Card {
  let tone: Tone;
  let summary: string;
  const { accepted, submitted } = result;
  if (submitted && accepted !== false) {
    tone = "ok";
    summary = "Scan submitted to the queue.";
    const { status, scan_number: number } = result;
    if (status === "running" && number !== undefined && number !== null) summary = `Scan #${number} is running.`;
    else if (number !== undefined && number !== null) summary = `Scan #${number} is in the queue.`;
    else if (status === "queued") summary = "Scan accepted and queued; it gets its number when it starts.";
    else if (status === "undecided") {
      tone = "warn";
      summary = "Submitted, but the scan server has not answered yet.";
    } else if (status) summary += ` Status: ${status}.`;
  } else if (submitted && accepted === false) {
    tone = "error";
    summary = `The scan server rejected the scan: ${result.guard_message ?? ""}`.trim();
  } else {
    tone = "error";
    summary = String(result.error ?? "The scan was not submitted.");
  }
  const card = emptyCard("bec_run_scan", "Scan", summary, tone);
  card.code = { label: "Call", text: callText };
  for (const key of ["scan_number", "scan_id", "request_id"]) {
    if (result[key] !== undefined && result[key] !== null) {
      card.stats.push({ label: key.replace(/_/g, " "), value: String(result[key]).slice(0, 13) });
    }
  }
  return card;
}
