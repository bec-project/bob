import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// The open chat's title, the MCP servers' status, the chat tools and the pickers for the next
// message: agent, model and thinking.
Rectangle {
    id: header

    implicitHeight: Math.max(52, row.implicitHeight + 16)
    color: Theme.surface

    Rectangle {
        anchors.bottom: parent.bottom
        width: parent.width
        height: 1
        color: Theme.border
    }

    RowLayout {
        id: row
        anchors.fill: parent
        anchors.leftMargin: 16
        anchors.rightMargin: 16
        spacing: 10

        IconButton {
            iconName: "sidebar"
            iconSize: 17
            tip: qsTr("Toggle chat list")
            onClicked: Prefs.sidebar = !Prefs.sidebar
        }
        Text {
            Layout.fillWidth: true
            Layout.minimumWidth: 60
            text: Chat.title
            color: Theme.text
            font.pixelSize: 15
            font.weight: Font.DemiBold
            elide: Text.ElideRight
        }

        // the controls wrap onto more lines when the window is narrow
        Item {
            Layout.preferredWidth: Math.min(controls.natural, header.width - 32 - 30 - 10 - 80 - 10)
            Layout.preferredHeight: controls.implicitHeight
            Flow {
                id: controls
                // the width the controls take on one line
                readonly property real natural: {
                    let width = 0
                    for (const child of children) {
                        if (child.visible)
                            width += child.implicitWidth + spacing
                    }
                    return Math.max(0, width - spacing)
                }
                width: parent.width
                spacing: 10
                // laid out from the right: the last control comes first
                layoutDirection: Qt.RightToLeft

                ToggleSwitch {
                    trackLast: true
                    iconName: "brain"
                    text: qsTr("Thinking")
                    enabled: Chat.thinkingVariant !== ""
                    checked: Chat.thinkingVariant === "" || Prefs.thinking
                    onToggled: Prefs.thinking = checked
                    height: 30
                    tip: Chat.thinkingVariant === ""
                        ? qsTr("This model has no no-thinking variant in OpenCode's configuration. Rebuild bob's image to add it.")
                        : Prefs.thinking ? qsTr("The model reasons before answering. Switch off for faster, shorter answers.")
                        : qsTr("Thinking is off (variant “%1”).").arg(Chat.thinkingVariant)
                }
                Item {
                    visible: Chat.modelOptions.length <= 1
                    implicitWidth: modelChip.implicitWidth
                    implicitHeight: 30
                    Chip {
                        id: modelChip
                        anchors.verticalCenter: parent.verticalCenter
                        text: Chat.activeModelLabel
                    }
                }
                AppComboBox {
                    visible: Chat.modelOptions.length > 1
                    model: Chat.modelOptions
                    textRole: "label"
                    valueRole: "key"
                    implicitHeight: 30
                    currentIndex: Chat.modelOptions.findIndex(option => option.key === Chat.activeModel)
                    tip: qsTr("Model for the next message")
                    onActivated: (index) => Chat.setModel(Chat.modelOptions[index].key)
                }
                AppComboBox {
                    visible: Chat.agentOptions.length > 1
                    model: Chat.agentOptions
                    implicitHeight: 30
                    currentIndex: Chat.agentOptions.indexOf(Chat.activeAgent)
                    tip: qsTr("Agent: build can change files, plan only reads")
                    onActivated: (index) => Chat.setAgent(Chat.agentOptions[index])
                }
                // chat tools
                Row {
                    visible: Chat.currentId !== "" && Chat.hasMessages
                    spacing: 2
                    IconButton {
                        iconName: "fork"
                        tip: qsTr("Fork: a copy of this chat to continue in a new direction")
                        onClicked: Chat.fork("")
                    }
                    IconButton {
                        iconName: "copy"
                        iconSize: 15
                        tip: qsTr("Copy the chat as Markdown")
                        onClicked: Chat.copyChat()
                    }
                    IconButton {
                        iconName: "download"
                        tip: qsTr("Export the chat as a Markdown file")
                        onClicked: Chat.exportChat()
                    }
                    Item {
                        width: 6
                        height: 30
                        Rectangle {
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            width: 1
                            height: 20
                            color: Theme.border
                        }
                    }
                }
                // one chip per MCP server OpenCode uses; one that is not connected connects again
                // when clicked
                Repeater {
                    model: Chat.mcpServers.slice().reverse()
                    delegate: McpChip {
                        required property var modelData
                        server: modelData
                    }
                }
            }
        }
    }
}
