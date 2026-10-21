import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The chat: the chat list, the open chat with its composer, and the side panels.
Item {
    id: view

    signal settingsRequested()

    RowLayout {
        anchors.fill: parent
        spacing: 0

        Sidebar {
            visible: Prefs.sidebar
            Layout.fillHeight: true
            Layout.preferredWidth: width
            onSettingsRequested: view.settingsRequested()
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            Header {
                Layout.fillWidth: true
            }
            // the broker ended the lease on the bob this window uses
            Banner {
                visible: Chat.lostReason !== ""
                Layout.fillWidth: true
                text: Chat.lostReason
                AppButton {
                    text: qsTr("Join again")
                    small: true
                    onClicked: Chat.joinBob()
                }
                AppButton {
                    text: qsTr("Switch bob…")
                    small: true
                    variant: "ghost"
                    onClicked: Chat.leaveBob()
                }
            }
            UsageBar {
                Layout.fillWidth: true
            }
            ThreadView {
                Layout.fillWidth: true
                Layout.fillHeight: true
            }
            Todos {
                Layout.fillWidth: true
            }
            Requests {
                Layout.fillWidth: true
            }
            Composer {
                Layout.fillWidth: true
            }
        }

        ChangesPanel {
            visible: Prefs.panel === "core/changes"
            Layout.fillHeight: true
            Layout.preferredWidth: width
        }

        // the panel rail
        Rectangle {
            Layout.fillHeight: true
            Layout.preferredWidth: 42
            color: Theme.surface2
            Rectangle {
                width: 1
                height: parent.height
                color: Theme.border
            }
            Column {
                anchors.top: parent.top
                anchors.topMargin: 10
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: 4
                IconButton {
                    iconName: "file"
                    iconSize: 18
                    tip: qsTr("Workspace changes")
                    active: Prefs.panel === "core/changes"
                    onClicked: Prefs.panel = active ? "" : "core/changes"
                    Rectangle {
                        visible: Chat.changedFiles > 0
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.margins: -2
                        width: Math.max(16, badge.implicitWidth + 8)
                        height: 16
                        radius: 8
                        color: Theme.accent
                        Text {
                            id: badge
                            anchors.centerIn: parent
                            text: Chat.changedFiles
                            color: "#ffffff"
                            font.pixelSize: 10
                            font.bold: true
                        }
                    }
                }
            }
        }
    }
}
