import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// Under an answer: model, time, tokens, tools and cost, and copy, fork and undo once it is done.
Flow {
    id: footer

    required property var item
    readonly property double elapsed: Math.max(0, (item.live ? Clock.now : item.end) - item.start)

    spacing: 12

    Meta {
        visible: footer.item.model !== ""
        text: footer.item.model
        tip: footer.item.modelTitle
    }
    Meta {
        iconName: "clock"
        text: Desktop.formatDuration(footer.elapsed)
        tip: qsTr("Time from the prompt to the last answer")
    }
    Meta {
        visible: footer.item.tokens !== ""
        text: footer.item.tokens
        tip: footer.item.tokensTitle
    }
    Meta {
        visible: footer.item.reasoning !== ""
        iconName: "brain"
        text: footer.item.reasoning
    }
    Meta {
        visible: footer.item.toolCalls > 0
        iconName: "tool"
        text: footer.item.toolCalls
    }
    Meta {
        visible: footer.item.steps > 1
        text: qsTr("%1 steps").arg(footer.item.steps)
    }
    Meta {
        visible: footer.item.cost !== ""
        text: footer.item.cost
    }
    Row {
        visible: !footer.item.live
        spacing: 0
        CopyButton {
            visible: footer.item.answer !== ""
            copyText: footer.item.answer
        }
        IconButton {
            box: 26
            iconName: "fork"
            iconSize: 14
            tip: qsTr("Fork: a new chat with everything up to this answer")
            onClicked: Chat.fork(footer.item.nextID)
        }
        IconButton {
            visible: footer.item.userID !== ""
            enabled: !footer.item.busy
            box: 26
            iconName: "undo"
            iconSize: 14
            tip: qsTr("Undo this message and everything after it, including file changes")
            onClicked: Chat.revert(footer.item.userID)
        }
    }

    component Meta: RowLayout {
        property string iconName
        property alias text: label.text
        property string tip
        height: 26
        spacing: 4
        Icon {
            visible: parent.iconName !== ""
            name: parent.iconName
            size: 12
            color: Theme.text3
        }
        Text {
            id: label
            color: Theme.text3
            font.pixelSize: 12
        }
        HoverHandler {
            id: metaHover
        }
        ToolTip.visible: metaHover.hovered && tip !== ""
        ToolTip.text: tip
        ToolTip.delay: 500
    }
}
