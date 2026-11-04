import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts

// Asks before something that cannot be undone.
Dialog {
    id: dialog

    property string text
    property string confirmText: qsTr("OK")

    parent: Overlay.overlay
    anchors.centerIn: parent
    width: Math.min(420, parent ? parent.width - 48 : 420)
    modal: true
    padding: 18
    topPadding: 14

    Overlay.modal: Rectangle {
        color: Theme.overlay
    }
    background: Rectangle {
        radius: 14
        color: Theme.surface
        border.color: Theme.border
    }
    header: Text {
        text: dialog.title
        padding: 18
        bottomPadding: 0
        color: Theme.text
        font.pixelSize: 16
        font.weight: Font.DemiBold
    }
    contentItem: Text {
        text: dialog.text
        textFormat: Text.PlainText
        color: Theme.text2
        wrapMode: Text.Wrap
    }
    footer: Item {
        implicitHeight: buttons.implicitHeight + 18
        RowLayout {
            id: buttons
            anchors.right: parent.right
            anchors.rightMargin: 18
            spacing: 8
            AppButton {
                text: qsTr("Cancel")
                onClicked: dialog.reject()
            }
            AppButton {
                text: dialog.confirmText
                variant: "danger"
                onClicked: dialog.accept()
            }
        }
    }
}
