import QtQuick
import QtQuick.Controls.Basic
import Bob.Backend

// The open chat. It stays at the newest message while the answer grows, unless the user scrolled
// up; then "Latest" brings it back.
Item {
    id: thread

    // follow new content: true until the user scrolls away from the end
    property bool pinned: true
    property bool autoScrolling: false

    function toEnd() {
        autoScrolling = true
        list.positionViewAtEnd()
        autoScrolling = false
    }

    // another chat opens at its end
    readonly property string sessionId: Chat.currentId
    onSessionIdChanged: {
        pinned = true
        Qt.callLater(toEnd)
    }

    ListView {
        id: list
        anchors.fill: parent
        model: Chat.thread
        clip: true
        topMargin: 20
        bottomMargin: 32
        cacheBuffer: 1600
        boundsBehavior: Flickable.StopAtBounds
        reuseItems: false
        ScrollBar.vertical: ScrollBar {}

        delegate: Item {
            id: slot
            required property string key
            required property string kind
            required property var item
            required property int index

            width: list.width
            height: content.implicitHeight + content.gap

            ThreadItem {
                id: content
                x: Math.round((slot.width - width) / 2)
                width: Math.min(Theme.column, slot.width - 40)
                key: slot.key
                kind: slot.kind
                item: slot.item
                viewHeight: list.height
            }
        }

        onContentHeightChanged: if (thread.pinned) Qt.callLater(thread.toEnd)
        onHeightChanged: if (thread.pinned) Qt.callLater(thread.toEnd)
        onCountChanged: if (thread.pinned) Qt.callLater(thread.toEnd)
        onContentYChanged: {
            if (thread.autoScrolling)
                return
            const end = list.originY + list.contentHeight + list.bottomMargin - list.height
            thread.pinned = end - list.contentY < 80
        }
    }

    AppButton {
        visible: !thread.pinned && !list.atYEnd
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: 12
        text: qsTr("Latest")
        iconName: "down"
        small: true
        onClicked: {
            thread.pinned = true
            thread.toEnd()
        }
        background: Rectangle {
            radius: 13
            color: Theme.surface
            border.color: Theme.border
        }
    }
}
