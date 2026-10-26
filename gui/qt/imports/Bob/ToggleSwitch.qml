import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts

// An on/off switch with its label; the track comes first unless `trackLast`.
AbstractButton {
    id: control

    property string iconName
    property bool trackLast: false
    property string tip

    checkable: true
    hoverEnabled: true
    implicitWidth: row.implicitWidth
    implicitHeight: 24
    opacity: enabled ? 1 : 0.5
    font.pixelSize: 13

    ToolTip.visible: hovered && tip !== ""
    ToolTip.text: tip
    ToolTip.delay: 500

    contentItem: RowLayout {
        id: row
        spacing: 8
        LayoutMirroring.enabled: control.trackLast
        LayoutMirroring.childrenInherit: false

        Rectangle {
            implicitWidth: 30
            implicitHeight: 18
            radius: 9
            color: control.checked ? Theme.accent : Theme.borderStrong
            Behavior on color { ColorAnimation { duration: 150 } }
            Rectangle {
                width: 14
                height: 14
                radius: 7
                y: 2
                x: control.checked ? 14 : 2
                color: "#ffffff"
                Behavior on x { NumberAnimation { duration: 150 } }
            }
        }
        Icon {
            visible: control.iconName !== ""
            name: control.iconName
            size: 15
            color: Theme.text2
        }
        Text {
            text: control.text
            font: control.font
            color: Theme.text2
        }
    }
}
