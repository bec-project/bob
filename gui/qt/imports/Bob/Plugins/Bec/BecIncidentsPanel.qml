import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob

// Problems in BEC's log, grouped into incidents by the server's bec_log_analysis, refreshed every
// 30 s. Servers without the analyser fall back to bec_logs. Code buttons appear when the server
// runs in code mode.
Item {
    id: panel

    property var context
    readonly property var server: context ? context.server : null
    readonly property var windows: [{ label: qsTr("15 min"), minutes: 15 }, { label: qsTr("1 h"), minutes: 60 },
                                    { label: qsTr("6 h"), minutes: 360 }, { label: qsTr("All cached"), minutes: 0 }]
    property int minutes: 60
    property string level: "error"
    property var tools: ({})
    property var card
    property string error
    property bool loading: false
    // {title, busy, error, card} of what Show records / Show code opened
    property var detail
    readonly property var actions: ({
        ask: text => panel.context.insertPrompt(text),
        showRecords: record => panel.show(qsTr("Records around #%1").arg(record), "bec_log_range", { around: record, before: 8, after: 8 }),
        showCode: panel.tools.bec_code_traceback ? record => panel.show(qsTr("Code behind record #%1").arg(record), "bec_code_traceback", { record: record })
                                                 : undefined
    })

    function load() {
        if (!server || loading)
            return
        loading = true
        BecServer.tools(server, (available, failed) => {
            if (failed) {
                loading = false
                error = failed
                return
            }
            tools = available
            const tool = available.bec_log_analysis ? "bec_log_analysis" : "bec_logs"
            const args = tool === "bec_log_analysis" ? { level: level, limit: 20 } : { level: level }
            if (tool === "bec_log_analysis" && minutes)
                args.minutes = minutes
            BecServer.callResult(server, tool, args, (result, failed) => {
                loading = false
                error = failed
                if (!failed)
                    card = BecCards.buildCard(tool, result)
            })
        })
    }
    function show(title, tool, args) {
        detail = { title: title, busy: true }
        BecServer.callResult(server, tool, args, (result, failed) => {
            if (!detail || detail.title !== title)
                return
            detail = failed ? { title: title, busy: false, error: failed }
                            : { title: title, busy: false, card: BecCards.buildCard(tool, result) }
        })
    }

    // a newly opened detail scrolls into view
    readonly property string detailTitle: detail ? detail.title : ""
    onDetailTitleChanged: if (detailTitle) Qt.callLater(() => {
        scroll.to = Math.min(detailBox.y, Math.max(0, flick.contentHeight - flick.height))
        scroll.restart()
    })

    onMinutesChanged: load()
    onLevelChanged: load()
    onServerChanged: load()
    Component.onCompleted: load()
    Timer {
        interval: 30000
        repeat: true
        running: panel.visible
        onTriggered: panel.load()
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        RowLayout {
            Layout.fillWidth: true
            Layout.margins: 8
            Layout.leftMargin: 12
            spacing: 8
            AppComboBox {
                model: panel.windows
                textRole: "label"
                currentIndex: panel.windows.findIndex(w => w.minutes === panel.minutes)
                onActivated: (index) => panel.minutes = panel.windows[index].minutes
            }
            AppComboBox {
                model: [qsTr("Errors"), qsTr("Warnings too")]
                currentIndex: panel.level === "error" ? 0 : 1
                onActivated: (index) => panel.level = index === 0 ? "error" : "warning"
            }
            Item { Layout.fillWidth: true }
            Spinner { visible: panel.loading }
            IconButton {
                iconName: "refresh"
                iconSize: 14
                tip: qsTr("Analyse again")
                enabled: !panel.loading
                onClicked: panel.load()
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
            NumberAnimation on contentY {
                id: scroll
                running: false
                duration: 200
                easing.type: Easing.OutCubic
            }

            ColumnLayout {
                id: body
                x: 12
                y: 12
                width: flick.width - 24
                spacing: 10

                Text {
                    visible: !panel.tools.bec_code_traceback && !!panel.tools.bec_log_analysis
                    Layout.fillWidth: true
                    text: qsTr("Start the MCP server with <code>--mode code</code> to see the code behind tracebacks.")
                    textFormat: Text.StyledText
                    color: Theme.text3
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
                // what Show records or Show code opened
                ColumnLayout {
                    id: detailBox
                    visible: !!panel.detail
                    Layout.fillWidth: true
                    spacing: 8
                    RowLayout {
                        Layout.fillWidth: true
                        Text {
                            Layout.fillWidth: true
                            text: panel.detail ? panel.detail.title : ""
                            color: Theme.text
                            font.weight: Font.DemiBold
                        }
                        IconButton {
                            iconName: "x"
                            iconSize: 13
                            tip: qsTr("Close")
                            onClicked: panel.detail = undefined
                        }
                    }
                    RowLayout {
                        visible: !!panel.detail && !!panel.detail.busy
                        spacing: 8
                        Spinner {}
                        Text { text: qsTr("Loading…"); color: Theme.text3; font.pixelSize: 13 }
                    }
                    Text {
                        visible: !!panel.detail && !!panel.detail.error
                        Layout.fillWidth: true
                        text: panel.detail && panel.detail.error ? panel.detail.error : ""
                        color: Theme.error
                        wrapMode: Text.Wrap
                    }
                    // a traceback under its summary, records on their own, anything else as a card
                    Text {
                        visible: !!panel.detail && !!panel.detail.card && !!panel.detail.card.frames
                        Layout.fillWidth: true
                        text: panel.detail && panel.detail.card ? panel.detail.card.summary ?? "" : ""
                        color: Theme.text2
                        font.pixelSize: 12
                        wrapMode: Text.Wrap
                    }
                    Loader {
                        active: !!panel.detail && !!panel.detail.card
                        Layout.fillWidth: true
                        sourceComponent: !active ? null : panel.detail.card.frames ? traceback
                                       : panel.detail.card.records ? records : detailCard
                    }
                    Component {
                        id: traceback
                        BecTraceback {
                            frames: panel.detail.card.frames
                            exceptions: panel.detail.card.exceptions ?? []
                        }
                    }
                    Component {
                        id: records
                        BecLogRecords {
                            records: panel.detail.card.records
                            actions: panel.actions
                        }
                    }
                    Component {
                        id: detailCard
                        BecCard {
                            card: panel.detail.card
                            compact: true
                            actions: panel.actions
                        }
                    }
                    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
                }
                Text {
                    visible: panel.error !== ""
                    Layout.fillWidth: true
                    text: panel.error
                    color: Theme.error
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: !!panel.card
                    Layout.fillWidth: true
                    text: panel.card ? panel.card.summary : ""
                    color: Theme.text2
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                Loader {
                    active: !!panel.card
                    Layout.fillWidth: true
                    sourceComponent: BecCard {
                        card: Object.assign({}, panel.card, { findings: [] })
                        compact: true
                        actions: panel.actions
                    }
                }
            }
        }
    }
}
