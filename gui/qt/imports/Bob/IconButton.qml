import QtQuick
import QtQuick.Controls.Basic

// A square button with an icon only; `tip` says what it does.
AbstractButton {
    id: control

    property string iconName
    property int iconSize: 16
    property string tip
    property bool active: false
    property int box: 30

    implicitWidth: box
    implicitHeight: box
    hoverEnabled: true
    focusPolicy: Qt.TabFocus
    opacity: enabled ? 1 : 0.4

    ToolTip.visible: hovered && tip !== ""
    ToolTip.text: tip
    ToolTip.delay: 500

    contentItem: Item {
        Icon {
            anchors.centerIn: parent
            name: control.iconName
            size: control.iconSize
            color: control.active ? Theme.accentText
                : control.hovered && control.enabled ? Theme.text : Theme.text2
        }
    }
    background: Rectangle {
        radius: Theme.radiusSmall
        color: control.active ? Theme.accentSoft
            : control.hovered && control.enabled ? Theme.surface3 : "transparent"
    }
}
