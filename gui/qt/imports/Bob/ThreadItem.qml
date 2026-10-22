import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// One row of a chat (see ThreadModel): its kind picks the view, its item carries what to show.
Item {
    id: row

    required property string key
    required property string kind
    required property var item
    // the visible height of the chat, for the empty chat's layout
    property real viewHeight: 400
    // the space below this row: a turn ends with more room than its parts
    readonly property int gap: kind === "footer" ? 26 : kind === "user" || kind === "undo" ? 14 : kind === "empty" ? 0 : 8

    implicitHeight: loader.item ? loader.item.implicitHeight : 0

    Loader {
        id: loader
        width: row.width
        opacity: row.item.undone && row.kind !== "undo" ? 0.45 : 1
        sourceComponent: {
            switch (row.kind) {
            case "loading":
            case "working": return working
            case "empty": return empty
            case "note": return note
            case "undo": return undo
            case "user": return user
            case "summary": return summary
            case "thinking": return thinking
            case "markdown": return markdown
            case "code": return code
            case "compaction": return compaction
            case "tool": return tool
            case "patch": return patch
            case "file": return file
            case "error": return error
            case "stopped": return stopped
            case "footer": return footer
            default: return null
            }
        }
    }

    Component {
        id: working
        RowLayout {
            spacing: 8
            Spinner {}
            Text {
                text: row.item.text ?? qsTr("Working…")
                color: Theme.text3
                font.pixelSize: 13
            }
        }
    }
    Component {
        id: empty
        EmptyChat {
            implicitHeight: Math.max(row.viewHeight * 0.6, 260)
        }
    }
    Component {
        id: note
        Text {
            text: row.item.text
            color: Theme.text3
            font.pixelSize: 13
        }
    }
    Component {
        id: undo
        Rectangle {
            implicitHeight: undoRow.implicitHeight + 16
            radius: Theme.radius
            color: Theme.surface
            border.color: Theme.borderStrong
            RowLayout {
                id: undoRow
                anchors.fill: parent
                anchors.leftMargin: 12
                anchors.rightMargin: 12
                spacing: 8
                Icon {
                    name: "undo"
                    size: 14
                    color: Theme.text2
                }
                Text {
                    Layout.fillWidth: true
                    text: qsTr("Undone from here, workspace files included. Your next message replaces it.")
                    color: Theme.text2
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                AppButton {
                    text: qsTr("Redo")
                    small: true
                    onClicked: Chat.unrevert()
                }
            }
        }
    }
    Component {
        id: user
        Item {
            implicitHeight: bubble.height
            Rectangle {
                id: bubble
                anchors.right: parent.right
                width: Math.min(parent.width * 0.8, userText.implicitWidth + 28)
                height: userText.implicitHeight + 18
                topLeftRadius: 14
                topRightRadius: 14
                bottomLeftRadius: 14
                bottomRightRadius: 4
                color: Theme.accent
                TextEdit {
                    id: userText
                    x: 14
                    y: 9
                    width: Math.min(row.width * 0.8 - 28, implicitWidth)
                    text: row.item.text
                    readOnly: true
                    selectByMouse: true
                    color: "#ffffff"
                    selectionColor: "#40ffffff"
                    font.pixelSize: 14
                    wrapMode: TextEdit.Wrap
                }
            }
        }
    }
    Component {
        id: summary
        Rectangle {
            implicitHeight: summaryColumn.implicitHeight + 16
            radius: Theme.radius
            color: Theme.surface
            border.color: Theme.border
            ColumnLayout {
                id: summaryColumn
                x: 12
                y: 8
                width: parent.width - 24
                spacing: 6
                Disclosure {
                    text: qsTr("Summary of the earlier chat")
                    open: row.item.expanded
                    onToggled: Chat.setExpanded(row.key, !row.item.expanded)
                }
                MarkdownView {
                    visible: row.item.expanded
                    Layout.fillWidth: true
                    markdown: row.item.text
                }
            }
        }
    }
    Component {
        id: thinking
        ThinkingBlock {
            rowKey: row.key
            item: row.item
        }
    }
    Component {
        id: markdown
        MarkdownView {
            markdown: row.item.text
        }
    }
    Component {
        id: code
        CodeBlock {
            text: row.item.text
            language: row.item.language
        }
    }
    Component {
        id: compaction
        RowLayout {
            spacing: 10
            Rectangle {
                Layout.fillWidth: true
                height: 1
                color: Theme.border
            }
            Text {
                text: row.item.auto ? qsTr("Chat compacted automatically: what follows continues from a summary.")
                                    : qsTr("Chat compacted: what follows continues from a summary.")
                color: Theme.text3
                font.pixelSize: 12
            }
            Rectangle {
                Layout.fillWidth: true
                height: 1
                color: Theme.border
            }
        }
    }
    Component {
        id: tool
        ToolCall {
            rowKey: row.key
            item: row.item
        }
    }
    Component {
        id: patch
        Item {
            implicitHeight: patchText.implicitHeight
            Text {
                id: patchText
                width: Math.min(implicitWidth, parent.width)
                text: (row.item.count === 1 ? qsTr("Changed 1 file: %1") : qsTr("Changed %1 files: %2").arg(row.item.count)).arg(row.item.names)
                textFormat: Text.PlainText
                color: Theme.accentText
                font.pixelSize: 13
                font.underline: patchHover.hovered
                elide: Text.ElideRight
                HoverHandler {
                    id: patchHover
                    cursorShape: Qt.PointingHandCursor
                }
                TapHandler {
                    onTapped: Prefs.panel = "core/changes"
                }
                ToolTip.visible: patchHover.hovered
                ToolTip.text: qsTr("Show the changes")
                ToolTip.delay: 500
            }
        }
    }
    Component {
        id: file
        Text {
            text: qsTr("Attached %1").arg(row.item.name)
            textFormat: Text.PlainText
            color: Theme.text3
            font.pixelSize: 13
        }
    }
    Component {
        id: error
        Rectangle {
            implicitHeight: errorText.implicitHeight + 16
            radius: Theme.radiusSmall
            color: Theme.errorSoft
            TextEdit {
                id: errorText
                x: 12
                y: 8
                width: parent.width - 24
                text: row.item.text
                readOnly: true
                selectByMouse: true
                color: Theme.error
                font.pixelSize: 13
                wrapMode: TextEdit.Wrap
            }
        }
    }
    Component {
        id: stopped
        Text {
            text: qsTr("Stopped.")
            color: Theme.text3
            font.pixelSize: 13
        }
    }
    Component {
        id: footer
        TurnFooter {
            item: row.item
        }
    }
}
