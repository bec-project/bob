import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The model's reasoning: open with a live timer while it thinks, closed after.
Rectangle {
    id: block

    required property string rowKey
    required property var item
    readonly property double elapsed: item.end > 0 ? item.end - item.start
        : item.running ? Clock.now - item.start : -1

    implicitHeight: column.implicitHeight + 12
    radius: Theme.radius
    color: Qt.rgba(Theme.thinkSoft.r, Theme.thinkSoft.g, Theme.thinkSoft.b, 0.55)
    border.color: Theme.thinkSoft

    ColumnLayout {
        id: column
        x: 10
        y: 6
        width: parent.width - 20
        spacing: 4
        Disclosure {
            Layout.fillWidth: true
            iconName: "brain"
            color: Theme.think
            open: block.item.expanded
            text: block.item.running ? qsTr("Thinking… %1").arg(Desktop.formatDuration(Math.max(0, block.elapsed)))
                : block.elapsed < 0 ? qsTr("Thought") : qsTr("Thought for %1").arg(Desktop.formatDuration(block.elapsed))
            onToggled: Chat.setExpanded(block.rowKey, !block.item.expanded)
            Dot {
                visible: block.item.running
                color: Theme.think
                pulse: true
            }
        }
        Flickable {
            visible: block.item.expanded
            Layout.fillWidth: true
            Layout.leftMargin: 20
            Layout.preferredHeight: Math.min(280, thought.implicitHeight)
            Layout.bottomMargin: 4
            contentHeight: thought.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            // a growing thought stays at its newest line
            onContentHeightChanged: if (block.item.running) contentY = Math.max(0, contentHeight - height)
            TextEdit {
                id: thought
                width: parent.width
                text: block.item.text || "…"
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.Wrap
                color: Theme.text2
                selectionColor: Theme.accentSoft
                font.pixelSize: 13
            }
        }
    }
}
