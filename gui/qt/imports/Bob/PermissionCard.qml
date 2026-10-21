import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// OpenCode asks before a shell command, an edit or a tool marked `ask`: allow once, always, or reject.
RequestCard {
    id: card

    required property var request
    property bool answering: false

    function answer(reply) {
        answering = true
        Chat.replyPermission(request.id, reply)
    }

    Keys.onPressed: (event) => {
        if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && (event.modifiers & Qt.ControlModifier)) {
            event.accepted = true
            answer("once")
        }
    }

    RowLayout {
        spacing: 8
        Icon {
            name: "shield"
            color: Theme.warn
        }
        Text {
            Layout.fillWidth: true
            text: card.request.title
            color: Theme.text
            font.pixelSize: 14
            font.weight: Font.DemiBold
            wrapMode: Text.Wrap
        }
    }
    Text {
        text: card.request.tool ? qsTr("Tool <code>%1</code>").arg(card.request.tool)
                                : qsTr("Permission <code>%1</code>").arg(card.request.permission)
        textFormat: Text.StyledText
        color: Theme.text3
        font.pixelSize: 13
    }
    Rectangle {
        visible: card.request.detail !== ""
        Layout.fillWidth: true
        implicitHeight: Math.min(200, detail.implicitHeight + 16)
        radius: Theme.radiusSmall
        color: Theme.surface2
        clip: true
        Flickable {
            anchors.fill: parent
            anchors.margins: 8
            contentHeight: detail.implicitHeight
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            TextEdit {
                id: detail
                width: parent.width
                text: card.request.detail
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.WrapAtWordBoundaryOrAnywhere
                color: Theme.text
                font.family: Theme.mono
                font.pixelSize: 12
            }
        }
    }
    RowLayout {
        Layout.topMargin: 4
        spacing: 8
        AppButton {
            text: qsTr("Allow once")
            variant: "primary"
            enabled: !card.answering
            focus: true
            tip: qsTr("Ctrl+Enter")
            onClicked: card.answer("once")
        }
        AppButton {
            visible: card.request.always.length > 0
            text: qsTr("Always allow")
            enabled: !card.answering
            tip: qsTr("Always allow: %1").arg(card.request.always.join(", "))
            onClicked: card.answer("always")
        }
        AppButton {
            text: qsTr("Reject")
            variant: "danger"
            enabled: !card.answering
            onClicked: card.answer("reject")
        }
    }
}
