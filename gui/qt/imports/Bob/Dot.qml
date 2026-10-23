import QtQuick

// A status dot; `pulse` makes it blink while something waits.
Rectangle {
    id: dot

    property bool pulse: false

    implicitWidth: 8
    implicitHeight: 8
    radius: width / 2
    color: Theme.text3

    SequentialAnimation on opacity {
        running: dot.pulse && dot.visible
        loops: Animation.Infinite
        onRunningChanged: if (!running) dot.opacity = 1
        NumberAnimation { to: 0.35; duration: 600; easing.type: Easing.InOutQuad }
        NumberAnimation { to: 1; duration: 600; easing.type: Easing.InOutQuad }
    }
}
