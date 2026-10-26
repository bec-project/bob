import QtQuick
import Bob.Backend

// A fenced code block from model output, with a copy button on hover.
Rectangle {
    id: block

    property string text
    property string language

    implicitHeight: code.implicitHeight + 24
    radius: Theme.radiusSmall
    color: Theme.surface2
    border.color: Theme.border

    HoverHandler {
        id: hover
    }

    TextEdit {
        id: code
        x: 14
        y: 12
        width: parent.width - 28
        text: block.text
        readOnly: true
        selectByMouse: true
        textFormat: TextEdit.PlainText
        wrapMode: TextEdit.WrapAtWordBoundaryOrAnywhere
        color: Theme.text
        selectionColor: Theme.accentSoft
        selectedTextColor: Theme.text
        font.family: Theme.mono
        font.pixelSize: 13
    }
    Text {
        visible: block.language !== "" && !hover.hovered
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 6
        text: block.language
        color: Theme.text3
        font.pixelSize: 11
    }
    CopyButton {
        visible: hover.hovered
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 4
        copyText: block.text
    }
}
