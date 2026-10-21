import QtQuick
import QtQuick.Layouts
import Bob.Backend

// A side panel on the right: its head with a close button, its content, and a splitter that keeps
// its width under `key`.
Rectangle {
    id: frame

    required property string key
    property string title
    property string iconName
    property int initial: 340
    default property alias content: body.data

    // the width the user gave it
    readonly property int size: splitter.size

    width: size
    color: Theme.surface

    Rectangle {
        width: 1
        height: parent.height
        color: Theme.border
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.leftMargin: 1
        spacing: 0

        RowLayout {
            Layout.fillWidth: true
            Layout.preferredHeight: 52
            Layout.leftMargin: 12
            Layout.rightMargin: 8
            spacing: 8
            Icon {
                name: frame.iconName
                size: 18
                color: Theme.text2
            }
            Text {
                Layout.fillWidth: true
                text: frame.title
                color: Theme.text
                font.weight: Font.DemiBold
                elide: Text.ElideRight
            }
            IconButton {
                iconName: "x"
                iconSize: 15
                tip: qsTr("Close")
                onClicked: Prefs.panel = ""
            }
        }
        Rectangle {
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
        Item {
            id: body
            Layout.fillWidth: true
            Layout.fillHeight: true
        }
    }

    Splitter {
        id: splitter
        key: "panel:" + frame.key
        initial: frame.initial
        minimum: 280
        side: "left"
    }
}
