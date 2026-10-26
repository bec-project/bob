import QtQuick
import QtQuick.Layouts
import Bob.Backend

// The agent's plan for the open chat, above the composer while work is left.
Item {
    id: todos

    property bool open: true
    readonly property var list: Chat.todos
    readonly property int done: list.filter(todo => todo.status === "completed" || todo.status === "cancelled").length
    readonly property var current: list.find(todo => todo.status === "in_progress")

    visible: list.length > 0 && done < list.length
    implicitHeight: visible ? card.implicitHeight + 8 : 0

    Rectangle {
        id: card
        x: Math.round((parent.width - width) / 2)
        width: Math.min(Theme.column, parent.width - 40)
        implicitHeight: column.implicitHeight + 14
        radius: Theme.radius
        color: Theme.surface
        border.color: Theme.border

        ColumnLayout {
            id: column
            x: 12
            y: 7
            width: parent.width - 24
            spacing: 2
            Disclosure {
                Layout.fillWidth: true
                iconName: "list"
                color: Theme.text
                open: todos.open
                text: todos.current ? todos.current.content : qsTr("Plan")
                onToggled: todos.open = !todos.open
                Text {
                    text: qsTr("%1/%2 done").arg(todos.done).arg(todos.list.length)
                    color: Theme.text3
                    font.pixelSize: 13
                }
            }
            Repeater {
                model: todos.open ? todos.list : []
                delegate: RowLayout {
                    required property var modelData
                    readonly property bool finished: modelData.status === "completed" || modelData.status === "cancelled"
                    Layout.leftMargin: 22
                    spacing: 8
                    Item {
                        implicitWidth: 14
                        implicitHeight: 14
                        Icon {
                            visible: modelData.status === "completed"
                            anchors.centerIn: parent
                            name: "check"
                            size: 12
                            color: Theme.ok
                        }
                        Spinner {
                            visible: modelData.status === "in_progress"
                            anchors.centerIn: parent
                            size: 10
                            thickness: 1.5
                        }
                        Dot {
                            visible: modelData.status !== "completed" && modelData.status !== "in_progress"
                            anchors.centerIn: parent
                        }
                    }
                    Text {
                        Layout.fillWidth: true
                        text: modelData.content
                        textFormat: Text.PlainText
                        color: finished ? Theme.text3 : Theme.text
                        font.pixelSize: 13
                        font.strikeout: finished
                        font.weight: modelData.status === "in_progress" ? Font.Medium : Font.Normal
                        wrapMode: Text.Wrap
                    }
                }
            }
        }
    }
}
