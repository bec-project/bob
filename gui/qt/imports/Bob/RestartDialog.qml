import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// Asks, after a join, whether to restart bob when it already ran with another bob.toml than the
// one this app asked for (started by another app, or still lingering after this one left it).
Dialog {
    id: dialog

    readonly property int others: (Bob.status.consumers ?? 1) - 1
    readonly property string file: Desktop.fileName(Bob.status.config ?? "")
    readonly property bool wanted: Chat.phase === "ready" && Chat.restartOffer && Bob.state === "joined"

    onWantedChanged: wanted ? open() : close()
    Component.onCompleted: if (wanted) open()

    parent: Overlay.overlay
    anchors.centerIn: parent
    width: Math.min(560, parent ? parent.width - 48 : 560)
    modal: true
    padding: 18
    topPadding: 14
    closePolicy: Popup.CloseOnEscape
    onClosed: Chat.dismissRestart()

    Overlay.modal: Rectangle {
        color: Theme.overlay
    }
    background: Rectangle {
        radius: 14
        color: Theme.surface
        border.color: Theme.border
    }
    header: Text {
        text: qsTr("Restart bob with %1?").arg(dialog.file)
        padding: 18
        bottomPadding: 0
        color: Theme.text
        font.pixelSize: 16
        font.weight: Font.DemiBold
    }
    contentItem: ColumnLayout {
        spacing: 10
        GridLayout {
            Layout.fillWidth: true
            columns: 2
            columnSpacing: 14
            rowSpacing: 4
            Text {
                text: qsTr("Running")
                color: Theme.text3
                font.pixelSize: 13
            }
            Text {
                Layout.fillWidth: true
                text: Bob.status.runningConfig ?? ""
                color: Theme.text
                font.pixelSize: 13
                wrapMode: Text.WrapAnywhere
            }
            Text {
                text: qsTr("Asked for")
                color: Theme.text3
                font.pixelSize: 13
            }
            Text {
                Layout.fillWidth: true
                text: Bob.status.config ?? ""
                color: Theme.text
                font.pixelSize: 13
                wrapMode: Text.WrapAnywhere
            }
        }
        Text {
            Layout.fillWidth: true
            text: qsTr("bob already ran with another bob.toml, so this app joined it. Restarting stops it and starts it with %1.").arg(dialog.file)
                + " " + (dialog.others > 0
                    ? (dialog.others === 1 ? qsTr("1 other app uses it too: it loses bob, and an answer in progress is cut off.")
                                           : qsTr("%1 other apps use it too: they lose bob, and an answer in progress is cut off.").arg(dialog.others))
                    : qsTr("No other app used it when this one joined."))
                + " " + qsTr("Chats stay in bob's home volume.")
            color: Theme.text2
            font.pixelSize: 13
            wrapMode: Text.Wrap
        }
    }
    footer: Item {
        implicitHeight: buttons.implicitHeight + 18
        RowLayout {
            id: buttons
            anchors.right: parent.right
            anchors.rightMargin: 18
            spacing: 8
            AppButton {
                text: qsTr("Keep using it")
                onClicked: Chat.dismissRestart()
            }
            AppButton {
                text: qsTr("Restart with %1").arg(dialog.file)
                variant: "primary"
                onClicked: Chat.restartBob()
            }
        }
    }
}
