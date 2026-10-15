import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { becToolName, buildCard, parseToolResult, scanResultCard } from "./cards";
import { formatScanCall } from "./scanCall";

const fixtures = path.join(import.meta.dirname, "fixtures");
const output = (name: string) => readFileSync(path.join(fixtures, `${name}.json`), "utf8");

describe("becToolName", () => {
  it("strips the MCP server prefix OpenCode adds", () => {
    expect(becToolName("bec_mcp_bec_devices")).toBe("bec_devices");
    expect(becToolName("bec_devices")).toBe("bec_devices");
    expect(becToolName("bec_bec_device_read")).toBe("bec_device_read");
    expect(becToolName("srv_bec_device_diagnose")).toBe("bec_device_diagnose");
    expect(becToolName("bec_mcp_mcp_status")).toBe("mcp_status");
  });

  it("ignores tools that are not BEC's", () => {
    expect(becToolName("bash")).toBeUndefined();
    expect(becToolName("other_devices")).toBeUndefined();
  });
});

describe("parseToolResult", () => {
  it("reads the server's JSON text", () => {
    expect(parseToolResult(output("bec_queue"))?.summary).toBeTruthy();
  });

  it("rejects text that is not a tool result", () => {
    expect(parseToolResult("total 8\ndrwxr-xr-x")).toBeUndefined();
    expect(parseToolResult('{"no":"summary"}')).toBeUndefined();
    expect(parseToolResult("[1,2]")).toBeUndefined();
  });
});

describe("buildCard", () => {
  it("draws every fixture without throwing", () => {
    for (const file of readdirSync(fixtures)) {
      const tool = file.replace(/\.json$/, "");
      const result = parseToolResult(output(tool));
      expect(result, file).toBeDefined();
      const card = buildCard(tool, result!);
      expect(card.summary, file).toBe(result!.summary);
    }
  });

  it("shows a device reading against its limits", () => {
    const card = buildCard("bec_device_read", parseToolResult(output("bec_device_read"))!);
    expect(card.gauge?.label).toBe("samx");
    expect(card.gauge?.value).toBe("5.005 mm");
    expect(card.gauge?.low).toBe("-50");
    expect(card.gauge?.fraction).toBeGreaterThan(0.5);
    expect(card.table?.columns).toEqual(["Signal", "Value", "Age"]);
  });

  it("lists devices with a note when the page is partial", () => {
    const card = buildCard("bec_devices", parseToolResult(output("bec_devices"))!);
    expect(card.table?.rows).toHaveLength(5);
    expect(card.table?.note).toBe("Showing 1–5 of 216");
    expect(card.stats.map((s) => s.label)).toContain("positioner");
  });

  it("splits findings into text and next step", () => {
    const card = buildCard("bec_diagnose", {
      ok: true,
      summary: "s",
      findings: ["Warning: the queue is paused. Next: bec_queue()."],
    });
    expect(card.findings[0]).toMatchObject({ severity: "warning", tone: "warn", text: "the queue is paused.", next: "bec_queue()" });
    expect(card.tone).toBe("warn");
  });

  it("marks a failed call as an error", () => {
    expect(buildCard("bec_status", { ok: false, summary: "BEC is down" }).tone).toBe("error");
  });
});

describe("scan calls", () => {
  it("formats bec_run_scan arguments as a BEC client call", () => {
    expect(formatScanCall({ scan: "line_scan", args: ["samx", -1, 1], kwargs: { steps: 10, relative: true } })).toBe(
      "scans.line_scan(dev.samx, -1, 1, steps=10, relative=True)",
    );
    expect(formatScanCall({ scan_name: "acquire", args: "[]", kwargs: '{"exp_time": 0.1}' })).toBe(
      "scans.acquire(exp_time=0.1)",
    );
  });

  it("words the submission result", () => {
    const card = scanResultCard({ submitted: true, status: "running", scan_number: 12 }, "scans.acquire()");
    expect(card.summary).toBe("Scan #12 is running.");
    expect(scanResultCard({ submitted: true, accepted: false, guard_message: "locked" }, "x").tone).toBe("error");
  });
});

describe("log and code cards", () => {
  it("groups the log analysis into incidents with cause and suggestion", () => {
    const card = buildCard("bec_log_analysis", parseToolResult(output("bec_log_analysis"))!);
    expect(card.incidents?.length).toBeGreaterThan(0);
    const limits = card.incidents!.find((i) => i.title.startsWith("DeviceInstructionError"))!;
    expect(limits.likelyCause).toContain("software limits");
    expect(limits.devices).toEqual(["samx"]);
    expect(limits.origin?.where).toBe("bec_server/scan_server/scans/scan_status.py:183 (_raise_if_failed)");
    expect(limits.tracebackRecord).toBe(592);
    expect(limits.context.length).toBeGreaterThan(0);
    expect(card.stats.find((s) => s.label === "records cached")?.value).toBe("760 / 3000");
  });

  it("lists log records with their tracebacks", () => {
    const card = buildCard("bec_log_range", parseToolResult(output("bec_log_range"))!);
    expect(card.records?.map((r) => r.id)).toEqual([589, 590, 591, 592, 593, 594]);
    const failed = card.records!.find((r) => r.id === 589)!;
    expect(failed.tone).toBe("error");
    expect(failed.traceback?.origin).toContain("scan_status.py:183");
    expect(card.badges[0].text).toBe("records 589–594");
  });

  it("shows traceback frames with their code, raising line marked", () => {
    const card = buildCard("bec_code_traceback", parseToolResult(output("bec_code_traceback"))!);
    expect(card.frames).toHaveLength(12);
    const last = card.frames![11];
    expect(last.function).toBe("DirectScanWorker._handle_exception");
    expect(last.code.split("\n").some((line) => line.startsWith(">") && line.includes("raise ScanAbortion"))).toBe(true);
    expect(card.exceptions?.[1].relation).toBe("cause");
  });

  it("shows code and search matches", () => {
    const read = buildCard("bec_code_read", parseToolResult(output("bec_code_read"))!);
    expect(read.code?.label).toContain("ScanStatus._raise_if_failed");
    const search = buildCard("bec_code_search", parseToolResult(output("bec_code_search"))!);
    expect(search.table?.rows[0][0].text).toBe("ophyd_devices/ophyd_devices/sim/sim_positioner.py:174");
  });

  it("knows the new tools behind a server prefix", () => {
    expect(becToolName("bec_mcp_bec_log_analysis")).toBe("bec_log_analysis");
    expect(becToolName("bec_mcp_bec_code_traceback")).toBe("bec_code_traceback");
  });
});
