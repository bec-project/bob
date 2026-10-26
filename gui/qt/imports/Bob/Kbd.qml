import QtQuick

// A key, as in a keyboard shortcut.
Rectangle {
    property alias text: label.text

    implicitWidth: label.implicitWidth + 10
    implicitHeight: label.implicitHeight + 3
    radius: 4
    color: Theme.surface
    border.color: Theme.border

    Text {
        id: label
        anchors.centerIn: parent
        color: Theme.text2
        font.pixelSize: 11
        font.family: Theme.mono
    }
}
