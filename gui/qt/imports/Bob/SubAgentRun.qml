import QtQuick
import Bob.Backend

// A sub-agent's work inside its task call: its steps, kept live by the event stream.
Item {
    id: run

    property string sessionId

    implicitHeight: steps.implicitHeight

    // a guide along the sub-agent's steps
    Rectangle {
        width: 2
        height: parent.height
        color: Theme.border
    }

    Column {
        id: steps
        x: 12
        width: parent.width - 12
        Repeater {
            model: run.sessionId !== "" ? Chat.stepsModel(run.sessionId) : null
            delegate: ThreadItem {
                width: steps.width
                height: implicitHeight + gap
            }
        }
    }
}
