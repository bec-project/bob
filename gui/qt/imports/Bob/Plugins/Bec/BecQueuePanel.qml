import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob

// The scan queue (refreshed every 5 s) and recent scans. Queue controls appear only when the
// server offers bec_queue_control (a DEV-mode server); every press asks first.
Item {
    id: panel

    property var context
    readonly property var server: context ? context.server : null
    readonly property var controls: [
        { action: "resume", label: qsTr("Resume"), confirm: qsTr("Resume the scan queue?") },
        { action: "halt", label: qsTr("Halt"), confirm: qsTr("Halt the running scan? It can be resumed.") },
        { action: "abort", label: qsTr("Abort"), confirm: qsTr("Abort the running scan? This hits whatever runs, whoever started it.") }
    ]
    property var queue
    property var history
    property string error
    property bool control: false
    property string notice
    property bool busy: false

    function loadQueue() {
        if (!server || busy)
            return
        busy = true
        BecServer.callResult(server, "bec_queue", { history: 3 }, (result, failed) => {
            busy = false
            error = failed
            if (!failed)
                queue = BecCards.buildCard("bec_queue", result)
        })
    }
    function loadHistory() {
        if (!server)
            return
        BecServer.callResult(server, "bec_scan_history", { limit: 10 }, (result, failed) => {
            if (failed)
                error = failed
            else
                history = BecCards.buildCard("bec_scan_history", result)
        })
    }
    function load() {
        loadQueue()
        loadHistory()
        // queue controls only when this server offers them; reads and controls go to the same server
        if (server)
            BecServer.tools(server, tools => control = !!tools.bec_queue_control)
    }
    function run(action) {
        server.call("bec_queue_control", { action: action }, (result, failed) => {
            const refused = result && typeof result === "object" ? result.error : undefined
            notice = failed ? failed : refused ? qsTr("The queue did not take “%1”: %2").arg(action).arg(String(refused))
                                               : qsTr("Sent “%1” to the queue.").arg(action)
            loadQueue()
        })
    }

    onServerChanged: load()
    Component.onCompleted: load()
    Timer {
        interval: 5000
        repeat: true
        running: panel.visible
        onTriggered: panel.loadQueue()
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        RowLayout {
            Layout.fillWidth: true
            Layout.margins: 8
            Layout.leftMargin: 12
            spacing: 6
            Repeater {
                model: panel.control ? panel.controls : []
                delegate: AppButton {
                    required property var modelData
                    text: modelData.label
                    small: true
                    variant: modelData.action === "abort" ? "danger" : "default"
                    onClicked: {
                        confirm.action = modelData.action
                        confirm.text = modelData.confirm
                        confirm.confirmText = modelData.label
                        confirm.open()
                    }
                }
            }
            Text {
                visible: !panel.control
                text: qsTr("Read-only: this server offers no queue control.")
                color: Theme.text3
                font.pixelSize: 12
            }
            Item { Layout.fillWidth: true }
            IconButton {
                iconName: "refresh"
                iconSize: 14
                tip: qsTr("Refresh")
                onClicked: {
                    panel.loadQueue()
                    panel.loadHistory()
                }
            }
        }
        Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }

        Flickable {
            id: flick
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            contentHeight: body.implicitHeight + 24
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar {}

            ColumnLayout {
                id: body
                x: 12
                y: 12
                width: flick.width - 24
                spacing: 10
                Text {
                    visible: panel.notice !== ""
                    Layout.fillWidth: true
                    text: panel.notice
                    color: Theme.text2
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: panel.error !== ""
                    Layout.fillWidth: true
                    text: panel.error
                    color: Theme.error
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: !!panel.queue
                    text: qsTr("Queue")
                    color: Theme.text
                    font.weight: Font.DemiBold
                }
                Text {
                    visible: !!panel.queue
                    Layout.fillWidth: true
                    text: panel.queue ? panel.queue.summary : ""
                    color: Theme.text2
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                Loader {
                    active: !!panel.queue
                    Layout.fillWidth: true
                    sourceComponent: BecCard {
                        card: panel.queue
                        compact: true
                        actions: ({ ask: text => panel.context.insertPrompt(text) })
                    }
                }
                Text {
                    visible: !!panel.history
                    Layout.topMargin: 6
                    text: qsTr("Recent scans")
                    color: Theme.text
                    font.weight: Font.DemiBold
                }
                Loader {
                    active: !!panel.history
                    Layout.fillWidth: true
                    sourceComponent: panel.history && panel.history.table && panel.history.table.rows.length ? historyCard : historyEmpty
                }
            }
        }
    }

    Component {
        id: historyCard
        BecCard {
            card: panel.history
            compact: true
        }
    }
    Component {
        id: historyEmpty
        Text {
            text: panel.history ? panel.history.summary : ""
            color: Theme.text3
            wrapMode: Text.Wrap
        }
    }
    ConfirmDialog {
        id: confirm
        property string action
        title: qsTr("BEC queue")
        onAccepted: panel.run(action)
    }
}
