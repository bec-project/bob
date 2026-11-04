import QtQuick
import Bob.Backend

// Copies `copyText`; shows a check for a moment after.
IconButton {
    id: button

    property string copyText
    property bool done: false

    iconName: done ? "check" : "copy"
    iconSize: 14
    box: 26
    tip: done ? qsTr("Copied") : qsTr("Copy")
    onClicked: {
        Desktop.copyText(copyText)
        done = true
        reset.restart()
    }

    Timer {
        id: reset
        interval: 1200
        onTriggered: button.done = false
    }
}
