import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// An MCP server's name, coloured by its state; clicking one that is not connected connects it again.
AbstractButton {
    id: chip

    required property var server
    readonly property string tone: server.tone.split(" ")[0]

    enabled: server.retry
    hoverEnabled: true
    implicitHeight: 30
    implicitWidth: chipRow.implicitWidth + 20
    onClicked: Chat.reconnectMcp(server.name)

    HoverHandler {
        id: hover
    }
    ToolTip.visible: hover.hovered
    ToolTip.text: server.detail
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
                text: chip.server.name
                color: chip.tone === "error" || chip.tone === "warn" ? Theme.tone(chip.tone) : Theme.text2
                font.pixelSize: 12
            }
        }
    }
}
