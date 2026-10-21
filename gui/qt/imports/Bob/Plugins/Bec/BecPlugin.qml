import QtQuick

// BEC: report cards for a BEC MCP server's tool results, readable scan approvals, and panels for
// devices, log incidents and the scan queue that read that server directly. bob.toml enables it
// per server with `plugin = "bec"`.
QtObject {
    id: plugin

    readonly property string pluginId: "bec"
    readonly property string name: "BEC"
    readonly property string description: qsTr("Report cards for BEC MCP results (logs, incidents and tracebacks included), readable scan approvals, and panels for devices, incidents and the queue.")
    readonly property var suggestions: [qsTr("Is the beamline healthy?"), qsTr("What went wrong in the BEC logs in the last hour?"),
                                        qsTr("Show the last five scans"), qsTr("What arguments does a line scan take?")]

    property Component card: Component { BecCard {} }
    readonly property var panels: [
        { id: "devices", title: qsTr("BEC devices"), icon: "chip", width: 340, component: devicesPanel },
        { id: "incidents", title: qsTr("BEC log incidents"), icon: "alert", width: 460, component: incidentsPanel },
        { id: "queue", title: qsTr("BEC queue and scans"), icon: "queue", width: 420, component: queuePanel }
    ]
    property Component devicesPanel: Component { BecDevicesPanel {} }
    property Component incidentsPanel: Component { BecIncidentsPanel {} }
    property Component queuePanel: Component { BecQueuePanel {} }
    property Component settings: Component { BecSettings {} }

    // In the chat, card buttons put a follow-up question into the composer.
    function chatActions(context) {
        return {
            ask: function (text) { context.insertPrompt(text) },
            showCode: function (record) { context.insertPrompt(qsTr("Show the code behind the traceback in BEC log record %1.").arg(record)) },
            showRecords: function (record) { context.insertPrompt(qsTr("Show the BEC log records around record %1.").arg(record)) }
        }
    }

    // How a call of one of the server's tools is drawn; `tool` is its name without the server's
    // prefix, `part` the call ({tool, state: {status, input, output}}). The cards are built in C++
    // (beccards.cpp); this adds the view that draws them.
    function describeTool(tool, part, context) {
        const view = BecCards.toolView(tool, part.state.status, part.state.input, part.state.output)
        if (view.card) {
            // a scan's card has no follow-up questions to ask
            const actions = tool === "bec_run_scan" ? undefined : chatActions(context)
            view.body = { component: card, properties: { card: view.card, compact: true, actions: actions } }
        }
        return view
    }

    // How a permission request for one of the server's tools is drawn: bec_run_scan as typed in the
    // BEC client. Allow once runs exactly this.
    function describePermission(tool, request, input) {
        if (tool !== "bec_run_scan")
            return undefined
        return { title: qsTr("Run this scan?"), text: BecCards.formatScanCall(input) }
    }
}
