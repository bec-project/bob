import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// Appearance, and the bob this window is connected to: leave it, or switch to another.
Dialog {
    id: dialog

    readonly property bool held: Bob.state === "joined" && Bob.status.name === Chat.connection.name

    parent: Overlay.overlay
    anchors.centerIn: parent
    width: Math.min(560, parent ? parent.width - 48 : 560)
    height: Math.min(implicitHeight, parent ? parent.height - 48 : implicitHeight)
    modal: true
    padding: 0

    Overlay.modal: Rectangle {
        color: Theme.overlay
    }
    background: Rectangle {
        radius: 14
        color: Theme.surface
        border.color: Theme.border
    }
    header: Item {
        implicitHeight: 52
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 18
            anchors.rightMargin: 10
            Text {
                Layout.fillWidth: true
                text: qsTr("Settings")
                color: Theme.text
                font.pixelSize: 16
                font.weight: Font.DemiBold
            }
            IconButton {
                iconName: "x"
                tip: qsTr("Close")
                onClicked: dialog.close()
            }
        }
        Rectangle {
            anchors.bottom: parent.bottom
            width: parent.width
            height: 1
            color: Theme.border
        }
    }

    contentItem: ScrollView {
        contentWidth: availableWidth
        ColumnLayout {
            width: parent.width
            spacing: 0

            Section {
                title: qsTr("Appearance")
                Rectangle {
                    implicitWidth: segments.implicitWidth + 4
                    implicitHeight: 30
                    radius: Theme.radiusSmall
                    color: Theme.surface2
                    Row {
                        id: segments
                        anchors.centerIn: parent
                        spacing: 0
                        Repeater {
                            model: [["system", qsTr("System")], ["light", qsTr("Light")], ["dark", qsTr("Dark")]]
                            delegate: AbstractButton {
                                id: segment
                                required property var modelData
                                readonly property bool on: Prefs.theme === modelData[0]
                                implicitWidth: label.implicitWidth + 24
                                implicitHeight: 26
                                onClicked: Prefs.theme = modelData[0]
                                background: Rectangle {
                                    radius: 5
                                    color: segment.on ? Theme.surface : "transparent"
                                    border.color: segment.on ? Theme.border : "transparent"
                                }
                                contentItem: Text {
                                    id: label
                                    text: segment.modelData[1]
                                    color: segment.on ? Theme.text : Theme.text2
                                    font.pixelSize: 13
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                }
                            }
                        }
                    }
                }
            }

            // one row per plugin instance: the plugin, the MCP server it serves, and its switch
            Section {
                visible: PluginRegistry.all.length > 0 || PluginRegistry.missing.length > 0
                title: qsTr("Plugins")
                Repeater {
                    model: PluginRegistry.all
                    delegate: RowLayout {
                        id: pluginRow
                        required property var modelData
                        readonly property bool on: {
                            PluginRegistry.revision
                            return Plugins.isEnabled(modelData.server)
                        }
                        Layout.fillWidth: true
                        spacing: 12
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 2
                            Text {
                                text: pluginRow.modelData.plugin.name + " · " + pluginRow.modelData.server
                                color: Theme.text
                                font.weight: Font.Medium
                            }
                            Text {
                                Layout.fillWidth: true
                                text: pluginRow.modelData.plugin.description
                                color: Theme.text3
                                font.pixelSize: 12
                                wrapMode: Text.Wrap
                            }
                            Loader {
                                active: pluginRow.on && !!pluginRow.modelData.plugin.settings
                                visible: active
                                Layout.fillWidth: true
                                Layout.topMargin: 6
                                sourceComponent: pluginRow.modelData.plugin.settings
                                onLoaded: item.context = PluginRegistry.context(pluginRow.modelData)
                            }
                        }
                        ToggleSwitch {
                            Layout.alignment: Qt.AlignTop
                            checked: pluginRow.on
                            onToggled: Plugins.setEnabled(pluginRow.modelData.server, checked)
                        }
                    }
                }
                Repeater {
                    model: PluginRegistry.missing
                    delegate: Text {
                        required property var modelData
                        Layout.fillWidth: true
                        text: modelData.error
                            ? qsTr("The plugin “%1” for %2 did not load: %3").arg(modelData.plugin).arg(modelData.server).arg(modelData.error)
                            : qsTr("bob.toml asks for the plugin “%1” for %2, which this app does not have.").arg(modelData.plugin).arg(modelData.server)
                        textFormat: Text.PlainText
                        color: Theme.text3
                        font.pixelSize: 12
                        wrapMode: Text.Wrap
                    }
                }
            }

            Section {
                title: qsTr("Connection")
                last: true
                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: 14
                    rowSpacing: 4
                    Repeater {
                        // key and value cells, in reading order
                        model: {
                            const connection = Chat.connection
                            const rows = [[qsTr("Server"), connection.url ?? ""]]
                            if (connection.name)
                                rows.push([qsTr("Sandbox"), connection.name + (connection.runtime ? " (%1)".arg(connection.runtime) : "")])
                            if (Chat.serverVersion)
                                rows.push([qsTr("OpenCode"), Chat.serverVersion])
                            rows.push([qsTr("App"), Bob.version])
                            for (const server of Chat.mcpServers) {
                                const version = Plugins.versions[server.name]
                                rows.push(["MCP " + server.name, server.status + (version ? " · " + version : "")
                                           + (server.error ? ": " + server.error : ""),
                                           server.status === "connected" ? "ok" : "warn"])
                            }
                            const cells = []
                            for (const row of rows)
                                cells.push({ text: row[0], key: true, tone: "" }, { text: row[1], key: false, tone: row[2] ?? "" })
                            return cells
                        }
                        delegate: Text {
                            required property var modelData
                            Layout.fillWidth: !modelData.key
                            text: modelData.text
                            textFormat: Text.PlainText
                            color: modelData.key ? Theme.text3 : modelData.tone ? Theme.tone(modelData.tone) : Theme.text
                            font.pixelSize: 13
                            wrapMode: modelData.key ? Text.NoWrap : Text.WrapAnywhere
                        }
                    }
                }
                RowLayout {
                    Layout.topMargin: 12
                    spacing: 8
                    AppButton {
                        text: dialog.held ? qsTr("Leave this bob") : qsTr("Switch bob…")
                        variant: dialog.held ? "danger" : "default"
                        tip: dialog.held ? qsTr("End this app's hold on %1; its chats stay in its home folder").arg(Bob.status.name) : ""
                        onClicked: {
                            dialog.close()
                            Chat.leaveBob()
                        }
                    }
                    AppButton {
                        visible: dialog.held && Bob.status.runningConfig !== Bob.status.config
                        text: qsTr("Restart with %1").arg(Desktop.fileName(Bob.status.config ?? ""))
                        tip: qsTr("Stop bob for every app that holds it and start it with %1").arg(Bob.status.config ?? "")
                        onClicked: {
                            dialog.close()
                            Chat.restartBob()
                        }
                    }
                }
                Text {
                    visible: dialog.held
                    Layout.topMargin: 8
                    Layout.fillWidth: true
                    text: {
                        if (!dialog.held)
                            return ""
                        let line = qsTr("This app holds your bob, started with %1. Leaving ends its hold and goes back to the connect screen; bob stops 90 seconds after no app holds it.").arg(Bob.status.runningConfig)
                        if (Bob.status.runningConfig !== Bob.status.config)
                            line += " " + qsTr("This app asked for %1; bob already ran with the other one, so the app joined that.").arg(Bob.status.config)
                        return line
                    }
                    color: Theme.text3
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
            }
        }
    }

    component Section: ColumnLayout {
        property string title
        property bool last: false
        default property alias content: body.data
        Layout.fillWidth: true
        spacing: 0
        ColumnLayout {
            id: body
            Layout.fillWidth: true
            Layout.margins: 18
            Layout.topMargin: 14
            Layout.bottomMargin: 14
            spacing: 8
            Text {
                text: parent.parent.title
                color: Theme.text
                font.pixelSize: 14
                font.weight: Font.DemiBold
            }
        }
        Rectangle {
            visible: !parent.last
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
    }
}
