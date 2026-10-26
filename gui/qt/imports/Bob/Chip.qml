import QtQuick

// A small rounded label; `tone` colours it (ok, info, warn, error).
Rectangle {
    id: chip

    property string text
    property string tone

    implicitWidth: label.implicitWidth + 16
    implicitHeight: 22
    radius: 11
    color: Theme.toneSoft(tone)

    Text {
        id: label
        anchors.centerIn: parent
        text: chip.text
        color: chip.tone ? Theme.tone(chip.tone) : Theme.text2
        font.pixelSize: 12
    }
}
