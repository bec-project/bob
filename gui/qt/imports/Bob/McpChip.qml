import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// An MCP server's name and version, coloured by its state; clicking one that is not connected
// connects it again. The version is what the server itself reports (MCP's serverInfo), asked
// again whenever it connects.
AbstractButton {
    id: chip

    required property var server
    readonly property string tone: server.tone.split(" ")[0]
    readonly property string version: Plugins.versions[server.name] ?? ""
    readonly property string url: (Plugins.servers.find(entry => entry.name === server.name) ?? {}).url ?? ""

    enabled: server.retry
    hoverEnabled: true
    implicitHeight: 30
    implicitWidth: chipRow.implicitWidth + 20
    onClicked: Chat.reconnectMcp(server.name)

    HoverHandler {
        id: hover
    }
    ToolTip.visible: hover.hovered
    ToolTip.text: server.detail + (version ? "\n" + qsTr("Version %1").arg(version) : "") + (url ? "\n" + url : "")
    ToolTip.delay: 300

    background: Rectangle {
        y: 2
        width: chip.width
        height: chip.height - 4
        radius: 13
        color: chip.hovered && chip.enabled ? Theme.surface3 : Theme.surface
        border.color: chip.tone === "error" || chip.tone === "warn" ? Theme.tone(chip.tone) : Theme.border
    }
    contentItem: Item {
        RowLayout {
            id: chipRow
            anchors.centerIn: parent
            spacing: 6
            Dot {
                color: Theme.tone(chip.tone)
                pulse: chip.server.tone.indexOf("pulse") >= 0
            }
            Text {
                text: chip.server.name + (chip.version ? " " + chip.version : "")
                color: chip.tone === "error" || chip.tone === "warn" ? Theme.tone(chip.tone) : Theme.text2
                font.pixelSize: 12
            }
        }
    }
}
