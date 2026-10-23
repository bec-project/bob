import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts

// A push button: default, primary, ghost or danger, normal or small, with an optional icon.
Button {
    id: control

    property string variant: "default"
    property bool small: false
    property string iconName
    property string tip
    readonly property color textColor: variant === "primary" ? "#ffffff"
        : variant === "danger" ? Theme.error : Theme.text

    implicitHeight: small ? 26 : 32
    leftPadding: small ? 8 : 12
    rightPadding: small ? 8 : 12
    topPadding: 0
    bottomPadding: 0
    font.pixelSize: small ? 13 : 14
    font.weight: Font.Medium
    hoverEnabled: true
    opacity: enabled ? 1 : 0.5

    ToolTip.visible: hovered && tip !== ""
    ToolTip.text: tip
    ToolTip.delay: 500

    // Enter presses a focused button too, not only Space
    Keys.onReturnPressed: if (enabled) clicked()
    Keys.onEnterPressed: if (enabled) clicked()

    contentItem: RowLayout {
        spacing: 6
        Icon {
            visible: control.iconName !== ""
            name: control.iconName
            size: control.small ? 12 : 14
            color: control.textColor
        }
        Text {
            visible: control.text !== ""
            text: control.text
            font: control.font
            color: control.textColor
            elide: Text.ElideRight
            Layout.fillWidth: true
        }
    }

    background: Rectangle {
        radius: Theme.radiusSmall
        color: {
            const hover = control.hovered && control.enabled
            switch (control.variant) {
            case "primary": return hover ? Qt.lighter(Theme.accent, 1.08) : Theme.accent
            case "ghost": return hover ? Theme.surface2 : "transparent"
            default: return hover ? Theme.surface2 : Theme.surface
            }
        }
        border.width: control.variant === "ghost" ? 0 : 1
        border.color: control.variant === "primary" ? Theme.accent
            : control.hovered && control.enabled ? Theme.borderStrong : Theme.border
        // the keyboard focus
        Rectangle {
            visible: control.visualFocus
            anchors.fill: parent
            anchors.margins: -3
            radius: parent.radius + 3
            color: "transparent"
            border.width: 2
            border.color: Theme.accent
            opacity: 0.6
        }
    }
}
