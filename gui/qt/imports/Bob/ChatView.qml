import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The chat: the chat list, the open chat with its composer, and the side panels. Below 900 px the
// chat list and the open side panel float over the chat instead of narrowing it.
Item {
    id: view

    signal settingsRequested()

    readonly property bool narrow: width < 900
    readonly property int railWidth: 42
    // at most this wide when it floats
    readonly property int floatingWidth: Math.min(340, width)

    RowLayout {
        anchors.fill: parent
        spacing: 0

        // the chat list's place, when there is room for it
        Item {
            visible: sidebar.visible && !view.narrow
            Layout.fillHeight: true
            Layout.preferredWidth: sidebar.width
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

        // the open side panel's place, when there is room for it
        Item {
            visible: panels.open && !view.narrow
            Layout.fillHeight: true
            Layout.preferredWidth: panels.width
        }

        // the panel rail
        Rectangle {
            Layout.fillHeight: true
            Layout.preferredWidth: view.railWidth
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

    Sidebar {
        id: sidebar
        visible: Prefs.sidebar
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.left: parent.left
        z: 2
        onSettingsRequested: view.settingsRequested()
    }
    Shade {
        visible: sidebar.visible && view.narrow
        anchors.left: sidebar.right
        z: 2
        toRight: true
    }

    // the open side panel: left of the rail, or at the window's right edge when it floats
    Item {
        id: panels
        readonly property bool open: changes.visible
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.right: parent.right
        anchors.rightMargin: view.narrow ? 0 : view.railWidth
        width: changes.visible ? changes.width : 0
        z: 2

        ChangesPanel {
            id: changes
            visible: Prefs.panel === "core/changes"
            height: parent.height
            width: view.narrow ? view.floatingWidth : size
        }
    }
    Shade {
        visible: panels.open && view.narrow
        anchors.right: panels.left
        z: 2
        toRight: false
    }

    // the soft edge of a floating chat list or panel, over the chat
    component Shade: Rectangle {
        property bool toRight
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        width: 12
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0; color: toRight ? Theme.shadow : "transparent" }
            GradientStop { position: 1; color: toRight ? "transparent" : Theme.shadow }
        }
    }
}
