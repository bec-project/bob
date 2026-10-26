import QtQuick
import QtQuick.Layouts

// A screen that only says what the app is busy with, and may offer to stop it.
Item {
    property alias text: label.text
    default property alias actions: row.data

    RowLayout {
        id: row
        anchors.centerIn: parent
        spacing: 8
        Spinner {}
        Text {
            id: label
            color: Theme.text3
            font.pixelSize: 13
        }
    }
}
