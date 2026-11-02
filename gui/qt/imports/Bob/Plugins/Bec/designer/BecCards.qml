pragma Singleton
import QtQuick

// Design Studio's stand-in for the plugin's C++ BecCards (beccards.h): no card for anything.
QtObject {
    function parseToolResult(output) { return undefined }
    function buildCard(tool, answer) {
        return { kind: tool, title: tool, summary: answer ? answer.summary || "" : "", tone: "neutral", badges: [], stats: [],
                 findings: [], links: [], lines: [] }
    }
    function scanResultCard(result, callText) { return buildCard("bec_run_scan", { summary: callText }) }
    function title(tool) { return undefined }
    function activity(tool) { return undefined }
    function formatScanCall(input) { return "scans." + ((input && (input.scan_name || input.scan)) || "?") + "(…)" }
    function toolView(tool, status, input, output) { return {} }
}
