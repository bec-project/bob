import QtQuick
import QtQuick.Controls.Basic
import Bob.Backend

// A drag handle along the edge of the chat list or a side panel. The width it sets is kept in
// Prefs under `key`; double-click goes back to the default. `side` is the edge it sits on:
// "right" for an element docked on the left (dragging right widens it), "left" for one on the
// right.
Item {
    id: splitter

    required property string key
    required property int initial
    property int minimum: 240
    property string side: "right"
    // the width to draw the element with
    readonly property int size: dragging >= 0 ? dragging : (Prefs.widths[key] ?? initial)

    property int dragging: -1
    property real startX
    property int startWidth

    width: 7
    ToolTip.visible: area.containsMouse && dragging < 0
    ToolTip.text: qsTr("Drag to resize; double-click for the default width")
    ToolTip.delay: 800
    anchors.top: parent.top
    anchors.bottom: parent.bottom
    x: side === "right" ? parent.width - 4 : -3
    z: 5

    Rectangle {
        x: 3
        width: 1
        height: parent.height
        color: area.containsMouse || splitter.dragging >= 0 ? Theme.accent : "transparent"
    }
    MouseArea {
        id: area
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.SplitHCursor
        onPressed: (mouse) => {
            const point = mapToItem(null, mouse.x, mouse.y)
            splitter.startX = point.x
            splitter.startWidth = splitter.size
            splitter.dragging = splitter.size
        }
        onPositionChanged: (mouse) => {
            if (splitter.dragging < 0)
                return
            const point = mapToItem(null, mouse.x, mouse.y)
            const delta = point.x - splitter.startX
            const window = splitter.Window.window
            splitter.dragging = Desktop.clampWidth(
                splitter.startWidth + (splitter.side === "left" ? -delta : delta), splitter.minimum,
                window ? window.width : 1280)
        }
        onReleased: {
            if (splitter.dragging >= 0)
                Prefs.setWidth(splitter.key, splitter.dragging)
            splitter.dragging = -1
        }
        // the release that follows must not store the width again
        onDoubleClicked: {
            splitter.dragging = -1
            Prefs.setWidth(splitter.key, -1)
        }
    }
}
