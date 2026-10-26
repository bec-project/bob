import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// One chat in the list. Double-click (or the pencil) renames it; the buttons float over the end of
// the row, so showing them changes neither its height nor its layout.
Item {
    id: row

    required property string sessionId
    required property string title
    required property double updated
    required property bool busy
    required property bool waiting
    readonly property bool active: sessionId === Chat.currentId
    property bool editing: false

    signal deleteRequested(string id, string title)

    implicitHeight: 34

    HoverHandler {
        id: hover
    }
    TapHandler {
        enabled: !row.editing
        onTapped: Chat.selectSession(row.sessionId)
        onDoubleTapped: row.editing = true
    }

    Rectangle {
        anchors.fill: parent
        radius: Theme.radiusSmall
        color: row.active ? Theme.surface : hover.hovered ? Theme.surface3 : "transparent"
        border.width: row.active ? 1 : 0
        border.color: Theme.border
    }

    RowLayout {
        visible: !row.editing
        anchors.fill: parent
        anchors.leftMargin: 8
        anchors.rightMargin: 8
        spacing: 8
        Dot {
            visible: row.waiting
            color: Theme.warn
            pulse: true
            ToolTip.visible: dotHover.hovered
            ToolTip.text: qsTr("Waiting for your answer")
            HoverHandler {
                id: dotHover
            }
        }
        Spinner {
            visible: row.busy && !row.waiting
            size: 12
        }
        Text {
            Layout.fillWidth: true
            text: row.title || qsTr("Untitled")
            textFormat: Text.PlainText
            color: Theme.text
            font.pixelSize: 14
            elide: Text.ElideRight
            HoverHandler {
                id: titleHover
            }
            ToolTip.visible: titleHover.hovered && !!row.title
            ToolTip.text: row.title
            ToolTip.delay: 800
        }
        Text {
            visible: !hover.hovered
            text: {
                Clock.now
                return Desktop.relativeTime(row.updated)
            }
            color: Theme.text3
            font.pixelSize: 12
        }
    }

    Rectangle {
        visible: hover.hovered && !row.editing
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        width: actions.implicitWidth + 20
        radius: Theme.radiusSmall
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0; color: "transparent" }
            GradientStop { position: 14 / (actions.implicitWidth + 20); color: row.active ? Theme.surface : Theme.surface3 }
        }
        Row {
            id: actions
            anchors.right: parent.right
            anchors.rightMargin: 4
            anchors.verticalCenter: parent.verticalCenter
            spacing: 2
            IconButton {
                box: 22
                iconName: "edit"
                iconSize: 14
                tip: qsTr("Rename")
                onClicked: row.editing = true
            }
            IconButton {
                box: 22
                iconName: "trash"
                iconSize: 14
                tip: qsTr("Delete")
                onClicked: row.deleteRequested(row.sessionId, row.title)
            }
        }
    }

    TextField {
        id: editor
        visible: row.editing
        anchors.fill: parent
        anchors.margins: 4
        text: row.title
        color: Theme.text
        font.pixelSize: 14
        leftPadding: 6
        background: Rectangle {
            radius: 4
            color: Theme.surface
            border.color: Theme.accent
        }
        onVisibleChanged: if (visible) {
            text = row.title
            forceActiveFocus()
            selectAll()
        }
        onAccepted: {
            row.editing = false
            if (text.trim() && text !== row.title)
                Chat.renameSession(row.sessionId, text.trim())
        }
        onActiveFocusChanged: if (!activeFocus) row.editing = false
        Keys.onEscapePressed: row.editing = false
    }
}
