import QtQuick
import QtQuick.Layouts

// A clickable heading with a chevron that turns when open.
Item {
    id: disclosure

    property string text
    property bool open: false
    property color color: Theme.text2
    property string iconName
    default property alias extra: extraRow.data

    signal toggled()

    implicitHeight: Math.max(20, headRow.implicitHeight)
    implicitWidth: headRow.implicitWidth

    RowLayout {
        id: headRow
        anchors.fill: parent
        spacing: 8
        Icon {
            name: "chevron"
            size: 13
            color: disclosure.color
            rotation: disclosure.open ? 90 : 0
            Behavior on rotation { NumberAnimation { duration: 150 } }
        }
        Icon {
            visible: disclosure.iconName !== ""
            name: disclosure.iconName
            size: 14
            color: disclosure.color
        }
        Text {
            text: disclosure.text
            color: disclosure.color
            font.pixelSize: 13
            font.weight: Font.Medium
        }
        RowLayout {
            id: extraRow
            spacing: 8
        }
        Item {
            Layout.fillWidth: true
        }
    }
    HoverHandler {
        cursorShape: Qt.PointingHandCursor
    }
    TapHandler {
        onTapped: disclosure.toggled()
    }
}
