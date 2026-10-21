pragma ComponentBehavior: Bound

import QtQuick
import Bob

// Numbered source lines as the BEC code tools print them, selectable, with a copy button on hover;
// the raising line starts with ">" and is marked.
Rectangle {
    id: block

    property string code
    // [start, end] of every marked line, as positions in `code`
    readonly property var marked: {
        const list = []
        let offset = 0
        for (const line of code.split("\n")) {
            if (line.indexOf(">") === 0)
                list.push([offset, offset + line.length])
            offset += line.length + 1
        }
        return list
    }

    implicitHeight: lines.implicitHeight + 12
    radius: Theme.radiusSmall
    color: Theme.surface2
    clip: true

    HoverHandler {
        id: hover
    }

    Repeater {
        model: block.marked
        delegate: Rectangle {
            required property var modelData
            // read again when the lines wrap differently
            readonly property rect first: {
                lines.width
                lines.contentHeight
                return lines.positionToRectangle(modelData[0])
            }
            readonly property rect last: {
                lines.width
                lines.contentHeight
                return lines.positionToRectangle(modelData[1])
            }
            y: lines.y + first.y
            width: block.width
            height: last.y + last.height - first.y
            color: Theme.errorSoft
        }
    }
    TextEdit {
        id: lines
        x: 10
        y: 6
        width: parent.width - 20
        text: block.code
        readOnly: true
        selectByMouse: true
        textFormat: TextEdit.PlainText
        wrapMode: TextEdit.WrapAnywhere
        color: Theme.text
        selectionColor: Theme.accentSoft
        selectedTextColor: Theme.text
        font.family: Theme.mono
        font.pixelSize: 12
    }
    CopyButton {
        visible: hover.hovered
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 4
        copyText: block.code
    }
}
