import QtQuick
import QtQuick.Layouts
import Bob

// A traceback from bec_code_traceback: the exception chain, then every frame with its code, the
// raising line marked; the last frame is open.
ColumnLayout {
    id: view

    property var frames: []
    property var exceptions: []

    spacing: 6

    Repeater {
        model: view.exceptions
        delegate: Text {
            required property var modelData
            required property int index
            Layout.fillWidth: true
            text: (index > 0 ? (modelData.relation === "context" ? qsTr("during handling, ") : qsTr("which caused ")) : "")
                  + "<code>" + modelData.text + "</code>"
            textFormat: Text.StyledText
            color: Theme.text
            font.pixelSize: 12
            wrapMode: Text.WrapAnywhere
        }
    }
    Repeater {
        model: view.frames
        delegate: ColumnLayout {
            id: frame
            required property var modelData
            required property int index
            property bool open: index === view.frames.length - 1
            readonly property bool mismatch: modelData.sourceCheck !== "" && modelData.sourceCheck !== "matches"
            Layout.fillWidth: true
            spacing: 4
            Disclosure {
                Layout.fillWidth: true
                open: frame.open
                color: Theme.text
                text: frame.modelData["function"]
                opacity: frame.modelData.code ? 1 : 0.6
                onToggled: if (frame.modelData.code) frame.open = !frame.open
                Text {
                    Layout.fillWidth: true
                    text: frame.modelData.where
                    elide: Text.ElideRight
                    color: Theme.text3
                    font.family: Theme.mono
                    font.pixelSize: 12
                }
                Chip {
                    visible: !frame.modelData.underRoots
                    text: qsTr("outside the code roots")
                }
                Chip {
                    visible: frame.mismatch
                    text: qsTr("source differs")
                    tone: "warn"
                }
            }
            BecCodeLines {
                visible: frame.open && !!frame.modelData.code
                Layout.fillWidth: true
                code: frame.modelData.code
            }
        }
    }
}
