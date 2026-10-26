import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The open chat's permission requests and questions, above the composer.
Item {
    id: requests

    visible: Chat.permissions.length + Chat.questions.length > 0
    implicitHeight: visible ? column.implicitHeight : 0

    // Ctrl+Enter (⌘↵ or ⌃↵ on macOS) allows the oldest request once, wherever the focus is: a
    // shortcut comes before the composer, whose Enter would otherwise send the draft
    Shortcut {
        id: allow
        sequences: Qt.platform.os === "osx" ? ["Ctrl+Return", "Ctrl+Enter", "Meta+Return", "Meta+Enter"]
                                            : ["Ctrl+Return", "Ctrl+Enter"]
        enabled: permissions.count > 0
        onActivated: {
            const card = permissions.itemAt(0)
            if (card && !card.answering)
                card.answer("once")
        }
    }

    ColumnLayout {
        id: column
        x: Math.round((parent.width - width) / 2)
        width: Math.min(Theme.column, parent.width - 40)
        spacing: 10

        Repeater {
            id: permissions
            model: Chat.permissions
            delegate: PermissionCard {
                required property var modelData
                required property int index
                Layout.fillWidth: true
                request: modelData
                shortcut: index === 0 ? allow.nativeText : ""
            }
        }
        Repeater {
            model: Chat.questions
            delegate: QuestionCard {
                required property var modelData
                Layout.fillWidth: true
                request: modelData
            }
        }
    }
}
