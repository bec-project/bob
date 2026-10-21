import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The open chat's permission requests and questions, above the composer.
Item {
    id: requests

    visible: Chat.permissions.length + Chat.questions.length > 0
    implicitHeight: visible ? column.implicitHeight : 0

    ColumnLayout {
        id: column
        x: Math.round((parent.width - width) / 2)
        width: Math.min(Theme.column, parent.width - 40)
        spacing: 10

        Repeater {
            model: Chat.permissions
            delegate: PermissionCard {
                required property var modelData
                Layout.fillWidth: true
                request: modelData
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
