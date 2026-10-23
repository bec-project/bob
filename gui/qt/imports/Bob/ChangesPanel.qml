import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// The files the open chat's prompts changed in the workspace, newest prompt first, each with Undo.
Rectangle {
    id: panel

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
            // a layout in a layout fills the height unless told otherwise
            Layout.fillHeight: false
            Layout.preferredHeight: 52
            Layout.leftMargin: 12
            Layout.rightMargin: 8
            spacing: 8
            Icon {
                name: "file"
                size: 18
                color: Theme.text2
            }
            Text {
                Layout.fillWidth: true
                text: qsTr("Workspace changes")
                color: Theme.text
                font.weight: Font.DemiBold
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
            Layout.preferredHeight: 1
            color: Theme.border
        }

        Text {
            visible: Chat.currentId === "" || Chat.changes.length === 0
            Layout.fillWidth: true
            Layout.margins: 14
            Layout.topMargin: 18
            text: Chat.currentId === "" ? qsTr("Open a chat to see the files it changed.")
                : qsTr("This chat has not changed any file in the workspace. Changes show here as the agent edits them, when the workspace is a git repository (OpenCode snapshots it with git).")
            color: Theme.text3
            font.pixelSize: 13
            wrapMode: Text.Wrap
        }

        Text {
            visible: Chat.changes.length > 0
            Layout.fillWidth: true
            leftPadding: 12
            topPadding: 8
            bottomPadding: 8
            text: qsTr("%1 %2 changed by %3 %4").arg(Chat.changedFiles).arg(Chat.changedFiles === 1 ? qsTr("file") : qsTr("files"))
                .arg(Chat.changes.length).arg(Chat.changes.length === 1 ? qsTr("message") : qsTr("messages"))
            color: Theme.text2
            font.pixelSize: 12
        }
        Rectangle {
            visible: Chat.changes.length > 0
            Layout.fillWidth: true
            Layout.preferredHeight: 1
            color: Theme.border
        }

        ListView {
            id: list
            visible: Chat.changes.length > 0
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: Chat.changes
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar {}
            delegate: ColumnLayout {
                id: change
                required property var modelData
                width: list.width
                spacing: 0
                opacity: modelData.undone ? 0.5 : 1

                RowLayout {
                    Layout.fillWidth: true
                    Layout.leftMargin: 12
                    Layout.rightMargin: 12
                    Layout.topMargin: 10
                    Layout.bottomMargin: 6
                    spacing: 8
                    Text {
                        Layout.fillWidth: true
                        text: change.modelData.prompt || qsTr("(no text)")
                        textFormat: Text.PlainText
                        color: Theme.text
                        font.pixelSize: 13
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                        maximumLineCount: 1
                        HoverHandler {
                            id: promptHover
                        }
                        ToolTip.visible: promptHover.hovered && !!change.modelData.prompt
                        ToolTip.text: change.modelData.prompt ?? ""
                        ToolTip.delay: 800
                    }
                    Chip {
                        visible: change.modelData.undone
                        text: qsTr("undone")
                    }
                    AppButton {
                        visible: !change.modelData.undone
                        text: qsTr("Undo")
                        iconName: "undo"
                        small: true
                        enabled: !Chat.busy
                        tip: qsTr("Undo this message and everything after it, including these file changes")
                        onClicked: Chat.revert(change.modelData.messageID)
                    }
                }
                Repeater {
                    model: change.modelData.files
                    delegate: FileDiff {
                        required property var modelData
                        Layout.fillWidth: true
                        diff: modelData
                    }
                }
            }
        }
        // keeps the message for an empty list at the top
        Item {
            visible: !list.visible
            Layout.fillHeight: true
        }
    }

    Splitter {
        id: splitter
        key: "panel:core/changes"
        initial: 460
        minimum: 280
        side: "left"
    }

    component FileDiff: ColumnLayout {
        id: fileDiff
        property var diff
        property bool open: false
        spacing: 0

        Rectangle {
            Layout.fillWidth: true
            implicitHeight: 34
            color: fileHover.hovered && fileDiff.diff.patch ? Theme.surface2 : "transparent"
            RowLayout {
                anchors.fill: parent
                anchors.leftMargin: 12
                anchors.rightMargin: 12
                spacing: 8
                Icon {
                    name: "chevron"
                    size: 12
                    color: Theme.text3
                    rotation: fileDiff.open ? 90 : 0
                }
                Text {
                    Layout.fillWidth: true
                    text: fileDiff.diff.file ?? qsTr("(unnamed)")
                    color: Theme.text
                    font.family: Theme.mono
                    font.pixelSize: 12
                    elide: Text.ElideLeft
                }
                Chip {
                    visible: !!fileDiff.diff.status && fileDiff.diff.status !== "modified"
                    text: fileDiff.diff.status ?? ""
                }
                Text {
                    text: "+" + (fileDiff.diff.additions ?? 0)
                    color: Theme.ok
                    font.pixelSize: 12
                }
                Text {
                    text: "−" + (fileDiff.diff.deletions ?? 0)
                    color: Theme.error
                    font.pixelSize: 12
                }
            }
            HoverHandler {
                id: fileHover
                cursorShape: fileDiff.diff.patch ? Qt.PointingHandCursor : Qt.ArrowCursor
            }
            TapHandler {
                enabled: !!fileDiff.diff.patch
                onTapped: fileDiff.open = !fileDiff.open
            }
        }
        Rectangle {
            visible: fileDiff.open
            Layout.fillWidth: true
            implicitHeight: patchText.implicitHeight + 8
            color: Theme.surface2
            TextEdit {
                id: patchText
                x: 12
                y: 4
                width: parent.width - 24
                readOnly: true
                selectByMouse: true
                textFormat: TextEdit.RichText
                wrapMode: TextEdit.WrapAnywhere
                color: Theme.text
                font.family: Theme.mono
                font.pixelSize: 11
                text: fileDiff.open ? Desktop.patchHtml(fileDiff.diff.patch ?? "", Theme.ok, Theme.error, Theme.info) : ""
            }
        }
        Rectangle {
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
    }
}
