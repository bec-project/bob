import QtQuick
import QtQuick.Layouts

// The frame of a request that waits for the user.
Rectangle {
    default property alias content: column.data

    implicitHeight: column.implicitHeight + 24
    radius: Theme.radius
    color: Theme.surface
    border.color: Theme.warn

    ColumnLayout {
        id: column
        x: 14
        y: 12
        width: parent.width - 28
        spacing: 6
    }
}
