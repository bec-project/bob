import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob

// Every BEC device of the server; a click reads it (bec_device_read).
Item {
    id: panel

    property var context
    readonly property var server: context ? context.server : null
    property var devices: []
    property string summary
    property string summaryTone
    property string error
    property bool loading: false
    property string selected
    property var reading: ({})
    readonly property var shown: {
        const needle = filter.text.trim().toLowerCase()
        const list = needle ? devices.filter(d => d.name.toLowerCase().indexOf(needle) !== -1
                                             || (d.device_class || "").toLowerCase().indexOf(needle) !== -1)
                            : devices
        return list.slice(0, 400)
    }

    function load() {
        if (!server)
            return
        loading = true
        error = ""
        BecServer.callResult(server, "bec_ping", {}, (ping, failed) => {
            summary = failed ? failed : ping.summary
            summaryTone = failed || ping.ok === false ? "error" : "ok"
        })
        const all = []
        const page = offset => BecServer.callResult(server, "bec_devices", { limit: 500, offset: offset }, (result, failed) => {
            if (failed) {
                error = failed
                loading = false
                return
            }
            const data = result.data || {}
            for (const device of data.devices || [])
                all.push(device)
            if (typeof data.next_offset === "number" && all.length < 5000) {
                page(data.next_offset)
                return
            }
            devices = all
            loading = false
        })
        page(0)
    }
    function read(device) {
        selected = device
        reading = { device: device, busy: true }
        BecServer.callResult(server, "bec_device_read", { device: device }, (result, failed) => {
            if (selected !== device)
                return
            reading = failed ? { device: device, busy: false, error: failed }
                             : { device: device, busy: false, card: BecCards.buildCard("bec_device_read", result) }
        })
    }

    Component.onCompleted: load()
    onServerChanged: load()

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        RowLayout {
            Layout.fillWidth: true
            Layout.leftMargin: 12
            Layout.rightMargin: 8
            Layout.topMargin: 6
            Layout.bottomMargin: 6
            spacing: 8
            Spinner {
                visible: panel.loading
            }
            Dot {
                visible: !panel.loading
                color: panel.error || panel.summaryTone === "error" ? Theme.error : panel.summaryTone === "ok" ? Theme.ok : Theme.text3
            }
            Text {
                Layout.fillWidth: true
                text: panel.error ? qsTr("Cannot reach the BEC MCP server") : panel.summary || qsTr("Connecting…")
                color: Theme.text2
                font.pixelSize: 12
                elide: Text.ElideRight
            }
            IconButton {
                iconName: "refresh"
                iconSize: 14
                tip: qsTr("Reload devices")
                enabled: !panel.loading
                onClicked: panel.load()
            }
        }
        Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
        AppTextField {
            id: filter
            Layout.fillWidth: true
            Layout.margins: 10
            implicitHeight: 30
            placeholderText: panel.devices.length ? qsTr("Filter %1 devices").arg(panel.devices.length) : qsTr("Filter devices")
        }
        Text {
            visible: panel.error !== ""
            Layout.fillWidth: true
            Layout.margins: 14
            text: panel.error
            color: Theme.error
            font.pixelSize: 13
            wrapMode: Text.Wrap
        }
        Text {
            visible: !panel.loading && !panel.error && panel.devices.length > 0 && panel.shown.length === 0
            Layout.margins: 14
            text: qsTr("No device matches “%1”.").arg(filter.text)
            color: Theme.text3
            font.pixelSize: 13
        }
        Text {
            visible: !panel.loading && !panel.error && panel.devices.length === 0
            Layout.margins: 14
            text: qsTr("BEC has no devices loaded.")
            color: Theme.text3
            font.pixelSize: 13
        }
        ListView {
            id: list
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: panel.shown
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar {}
            delegate: ColumnLayout {
                id: row
                required property var modelData
                readonly property bool open: panel.selected === modelData.name
                width: list.width
                spacing: 0
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: 30
                    color: row.open ? Theme.accentSoft : rowHover.hovered ? Theme.surface2 : "transparent"
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: 12
                        anchors.rightMargin: 12
                        spacing: 8
                        Dot {
                            color: row.modelData.enabled === false ? Theme.warn : Theme.ok
                        }
                        Text {
                            Layout.fillWidth: true
                            text: row.modelData.name
                            color: Theme.text
                            font.family: Theme.mono
                            font.pixelSize: 12
                            elide: Text.ElideRight
                        }
                        Text {
                            text: row.modelData.type ?? ""
                            color: Theme.text3
                            font.pixelSize: 12
                        }
                    }
                    HoverHandler {
                        id: rowHover
                        cursorShape: Qt.PointingHandCursor
                    }
                    TapHandler {
                        onTapped: row.open ? panel.selected = "" : panel.read(row.modelData.name)
                    }
                    ToolTip.visible: rowHover.hovered && !!row.modelData.device_class
                    ToolTip.text: row.modelData.device_class ?? ""
                    ToolTip.delay: 600
                }
                ColumnLayout {
                    visible: row.open && panel.reading.device === row.modelData.name
                    Layout.fillWidth: true
                    Layout.margins: 12
                    spacing: 8
                    RowLayout {
                        visible: !!panel.reading.busy
                        spacing: 8
                        Spinner {}
                        Text { text: qsTr("Reading %1…").arg(row.modelData.name); color: Theme.text3; font.pixelSize: 13 }
                    }
                    Text {
                        visible: !!panel.reading.error
                        Layout.fillWidth: true
                        text: panel.reading.error ?? ""
                        color: Theme.error
                        font.pixelSize: 13
                        wrapMode: Text.Wrap
                    }
                    Loader {
                        active: row.open && !!panel.reading.card
                        Layout.fillWidth: true
                        sourceComponent: BecCard {
                            card: panel.reading.card
                            compact: true
                        }
                    }
                    Flow {
                        Layout.fillWidth: true
                        spacing: 6
                        AppButton {
                            text: qsTr("Read again")
                            small: true
                            enabled: !panel.reading.busy
                            onClicked: panel.read(row.modelData.name)
                        }
                        AppButton {
                            text: qsTr("Ask about it")
                            small: true
                            onClicked: panel.context.insertPrompt(qsTr("What is the state of %1? ").arg(row.modelData.name))
                        }
                        AppButton {
                            text: qsTr("Diagnose")
                            small: true
                            onClicked: panel.context.insertPrompt(qsTr("Diagnose the device %1.").arg(row.modelData.name))
                        }
                    }
                }
            }
        }
    }
}
