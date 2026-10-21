import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// OpenCode asks before a shell command, an edit or a tool marked `ask`: allow once, always, or reject.
RequestCard {
    id: card

    required property var request
    property bool answering: false
    // the keys that allow this request once, on the oldest request only (see Requests.qml)
    property string shortcut: ""
    // how the plugin serving the tool's MCP server words the request, if one does
    readonly property var custom: {
        PluginRegistry.revision
        return PluginRegistry.enabled.length ? PluginRegistry.permissionView(request) ?? {} : {}
    }

    function answer(reply) {
        answering = true
        Chat.replyPermission(request.id, reply)
    }

    // a new request takes the keyboard, so Enter allows it once
    Component.onCompleted: allowOnce.forceActiveFocus(Qt.TabFocusReason)

    // the request stays open when the reply fails: answer it again
    Connections {
        target: Chat
        function onPermissionReplyFailed(id) {
            if (id === card.request.id)
                card.answering = false
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
            text: card.custom.title ?? card.request.title
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
    // a plugin's own view of the request
    Loader {
        active: !!card.custom.body
        visible: active
        Layout.fillWidth: true
        sourceComponent: card.custom.body ? card.custom.body.component : null
        onLoaded: {
            const properties = card.custom.body.properties || {}
            for (const key in properties)
                item[key] = properties[key]
        }
    }
    Rectangle {
        visible: !card.custom.body && (card.custom.text ?? card.request.detail) !== ""
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
                text: card.custom.text ?? card.request.detail
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
            id: allowOnce
            text: qsTr("Allow once")
            variant: "primary"
            enabled: !card.answering
            tip: card.shortcut
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
