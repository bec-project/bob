import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// The chat list: chats of this bob grouped by day, with search, rename and delete.
Rectangle {
    id: sidebar

    signal settingsRequested()

    width: splitter.size
    color: Theme.surface2

    Rectangle {
        anchors.right: parent.right
        width: 1
        height: parent.height
        color: Theme.border
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.rightMargin: 1
        spacing: 0

        RowLayout {
            Layout.fillWidth: true
            Layout.margins: 12
            Layout.bottomMargin: 8
            spacing: 8
            Image {
                source: "images/bob.png"
                // the full 128 px image, scaled down smoothly: a 20 px decode is blurry on HiDPI screens
                mipmap: true
                fillMode: Image.PreserveAspectFit
                Layout.preferredWidth: 20
                Layout.preferredHeight: 20
            }
            Text {
                text: "bob"
                color: Theme.text
                font.pixelSize: 15
                font.bold: true
            }
            Item {
                Layout.fillWidth: true
            }
            AppButton {
                text: qsTr("New chat")
                iconName: "plus"
                small: true
                tip: qsTr("New chat (Ctrl+Shift+O)")
                onClicked: Chat.selectSession("")
            }
        }
        AppTextField {
            Layout.fillWidth: true
            Layout.leftMargin: 12
            Layout.rightMargin: 12
            Layout.bottomMargin: 8
            implicitHeight: 30
            placeholderText: qsTr("Search chats")
            text: Chat.sessions.query
            onTextChanged: Chat.sessions.query = text
        }

        ListView {
            id: list
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: Chat.sessions
            leftMargin: 6
            rightMargin: 6
            bottomMargin: 8
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar {}
            section.property: "group"
            section.delegate: Text {
                required property string section
                width: list.width - 12
                leftPadding: 8
                topPadding: 10
                bottomPadding: 4
                text: section.toUpperCase()
                color: Theme.text3
                font.pixelSize: 11
                font.weight: Font.DemiBold
                font.letterSpacing: 0.4
            }
            delegate: SessionRow {
                width: list.width - 12
                onDeleteRequested: (id, title) => {
                    confirm.sessionId = id
                    confirm.sessionTitle = title
                    confirm.open()
                }
            }

            Text {
                visible: list.count === 0
                anchors.top: parent.top
                anchors.topMargin: 24
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Chat.sessions.query ? qsTr("No chat matches.") : qsTr("No chats in bob yet.")
                color: Theme.text3
                font.pixelSize: 13
            }
        }

        Rectangle {
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
        RowLayout {
            Layout.fillWidth: true
            Layout.leftMargin: 12
            Layout.rightMargin: 8
            Layout.topMargin: 6
            Layout.bottomMargin: 6
            spacing: 8
            Dot {
                color: Chat.stream === "open" ? Theme.ok : Chat.stream === "retrying" ? Theme.warn : Theme.error
                pulse: Chat.stream === "retrying"
            }
            Text {
                id: foot
                Layout.fillWidth: true
                text: {
                    const states = { open: qsTr("Connected"), retrying: qsTr("Reconnecting…"), connecting: qsTr("Connecting…") }
                    let line = states[Chat.stream] ?? qsTr("Offline")
                    if (Chat.connection.name)
                        line += " · " + Chat.connection.name
                    if (Chat.serverVersion)
                        line += " · OpenCode " + Chat.serverVersion
                    return line
                }
                color: Theme.text3
                font.pixelSize: 12
                elide: Text.ElideRight
                HoverHandler {
                    id: footHover
                }
                ToolTip.visible: footHover.hovered
                ToolTip.delay: 500
                ToolTip.text: (Chat.connection.url ?? "") + (Chat.streamDetail ? "\n" + Chat.streamDetail : "")
            }
            IconButton {
                iconName: "settings"
                tip: qsTr("Settings")
                onClicked: sidebar.settingsRequested()
            }
        }
    }

    Splitter {
        id: splitter
        key: "sidebar"
        initial: 264
        minimum: 200
        side: "right"
    }

    ConfirmDialog {
        id: confirm
        property string sessionId
        property string sessionTitle
        title: qsTr("Delete this chat?")
        text: qsTr("Delete “%1”? This removes it from bob's history.").arg(sessionTitle)
        confirmText: qsTr("Delete")
        onAccepted: Chat.deleteSession(sessionId)
    }
}
