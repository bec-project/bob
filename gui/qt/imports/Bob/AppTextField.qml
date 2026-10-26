import QtQuick
import QtQuick.Controls.Basic

// A one-line text input.
TextField {
    id: control

    property bool monospace: false

    implicitHeight: 34
    leftPadding: 10
    rightPadding: 10
    color: Theme.text
    placeholderTextColor: Theme.text3
    selectionColor: Theme.accentSoft
    selectedTextColor: Theme.text
    font.pixelSize: 14
    font.family: monospace ? Theme.mono : Qt.application.font.family

    background: Rectangle {
        radius: Theme.radiusSmall
        color: Theme.surface
        border.color: control.activeFocus ? Theme.accent : Theme.border
        Rectangle {
            visible: control.activeFocus
            anchors.fill: parent
            anchors.margins: -3
            z: -1
            radius: Theme.radiusSmall + 3
            color: "transparent"
            border.width: 3
            border.color: Theme.accentSoft
        }
    }
}
