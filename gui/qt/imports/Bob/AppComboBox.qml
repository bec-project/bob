import QtQuick
import QtQuick.Controls.Basic

// A small drop-down, for the model and agent pickers.
ComboBox {
    id: control

    property string tip

    implicitHeight: 28
    implicitWidth: Math.min(220, Math.max(90, contentWidth + 36))
    readonly property real contentWidth: metrics.advanceWidth
    font.pixelSize: 13
    hoverEnabled: true

    ToolTip.visible: hovered && tip !== "" && !popup.visible
    ToolTip.text: tip
    ToolTip.delay: 500

    TextMetrics {
        id: metrics
        font: control.font
        text: control.displayText
    }

    contentItem: Text {
        leftPadding: 8
        rightPadding: 24
        text: control.displayText
        font: control.font
        color: Theme.text
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
    indicator: Icon {
        x: control.width - width - 8
        y: (control.height - height) / 2
        name: "chevron"
        size: 12
        color: Theme.text3
        rotation: 90
    }
    background: Rectangle {
        radius: Theme.radiusSmall
        color: control.hovered ? Theme.surface2 : Theme.surface
        border.color: control.visualFocus ? Theme.accent : Theme.border
    }
    delegate: ItemDelegate {
        required property var modelData
        required property int index
        // the list inside the popup's padding
        width: control.popup.availableWidth
        height: 28
        highlighted: control.highlightedIndex === index
        contentItem: Text {
            text: control.textRole ? modelData[control.textRole] : modelData
            font: control.font
            color: Theme.text
            elide: Text.ElideRight
            verticalAlignment: Text.AlignVCenter
        }
        background: Rectangle {
            color: parent.highlighted ? Theme.accentSoft : "transparent"
            radius: 4
        }
    }
    popup: Popup {
        y: control.height + 2
        width: Math.max(control.width, 220)
        implicitHeight: Math.min(contentItem.implicitHeight + 8, 360)
        padding: 4
        contentItem: ListView {
            clip: true
            implicitHeight: contentHeight
            model: control.popup.visible ? control.delegateModel : null
            currentIndex: control.highlightedIndex
            ScrollIndicator.vertical: ScrollIndicator {}
        }
        background: Rectangle {
            radius: Theme.radiusSmall
            color: Theme.surface
            border.color: Theme.border
        }
    }
}
