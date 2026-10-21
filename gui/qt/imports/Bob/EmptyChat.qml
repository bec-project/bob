import QtQuick
import QtQuick.Layouts
import Bob.Backend

// A new chat: what bob can do, and a few prompts to start with.
Item {
    readonly property var suggestions: [qsTr("What can you do?"), qsTr("Summarise the files in the workspace"),
                                        qsTr("Explain the project layout")]

    ColumnLayout {
        anchors.centerIn: parent
        width: Math.min(620, parent.width)
        spacing: 14
        Text {
            Layout.alignment: Qt.AlignHCenter
            text: qsTr("What can I help with?")
            color: Theme.text
            font.pixelSize: 20
            font.weight: Font.DemiBold
        }
        Text {
            Layout.alignment: Qt.AlignHCenter
            Layout.fillWidth: true
            horizontalAlignment: Text.AlignHCenter
            text: qsTr("The assistant runs in bob's container, with its tools and MCP servers.")
            color: Theme.text3
            wrapMode: Text.Wrap
        }
        GridLayout {
            Layout.fillWidth: true
            columns: width >= 3 * 220 ? 3 : 1
            columnSpacing: 8
            rowSpacing: 8
            Repeater {
                model: suggestions
                delegate: Rectangle {
                    id: suggestion
                    required property string modelData
                    Layout.fillWidth: true
                    implicitHeight: label.implicitHeight + 20
                    radius: Theme.radius
                    color: Theme.surface
                    border.color: hover.hovered ? Theme.accent : Theme.border
                    Text {
                        id: label
                        anchors.fill: parent
                        anchors.margins: 10
                        anchors.leftMargin: 12
                        text: suggestion.modelData
                        color: hover.hovered ? Theme.text : Theme.text2
                        wrapMode: Text.Wrap
                    }
                    HoverHandler {
                        id: hover
                        cursorShape: Qt.PointingHandCursor
                    }
                    TapHandler {
                        onTapped: Chat.send(suggestion.modelData)
                    }
                }
            }
        }
    }
}
