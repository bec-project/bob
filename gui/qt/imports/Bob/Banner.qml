import QtQuick
import QtQuick.Layouts

// A strip across the chat, for something the user should know now (warn or error).
Rectangle {
    id: banner

    property string tone: "warn"
    property string text
    property string iconName: "alert"
    default property alias actions: actionRow.data

    implicitHeight: row.implicitHeight + 12
    color: Theme.toneSoft(tone)

    RowLayout {
        id: row
        anchors.fill: parent
        anchors.leftMargin: 16
        anchors.rightMargin: 16
        spacing: 8
        Icon {
            name: banner.iconName
            size: 14
            color: Theme.tone(banner.tone)
        }
        Text {
            Layout.fillWidth: true
            text: banner.text
            textFormat: Text.PlainText
            color: Theme.tone(banner.tone)
            font.pixelSize: 13
            wrapMode: Text.Wrap
        }
        RowLayout {
            id: actionRow
            spacing: 6
        }
    }
}
