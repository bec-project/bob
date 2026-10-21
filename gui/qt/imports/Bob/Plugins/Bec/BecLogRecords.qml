import QtQuick
import QtQuick.Layouts
import Bob

// Log records (bec_log_range), with their tracebacks; *Show code* where the server can show it.
ColumnLayout {
    id: view

    property var records: []
    property var actions

    spacing: 0

    Repeater {
        model: view.records
        delegate: ColumnLayout {
            id: record
            required property var modelData
            Layout.fillWidth: true
            Layout.bottomMargin: 6
            spacing: 2
            RowLayout {
                spacing: 6
                Text {
                    visible: record.modelData.id !== undefined
                    text: "#" + record.modelData.id
                    color: Theme.text3
                    font.pixelSize: 12
                }
                Text {
                    text: record.modelData.time
                    color: Theme.text3
                    font.pixelSize: 12
                    font.family: Theme.mono
                }
                Chip {
                    text: record.modelData.level
                    tone: record.modelData.tone === "neutral" ? "" : record.modelData.tone
                }
                Text {
                    text: record.modelData.service
                    color: Theme.text2
                    font.pixelSize: 12
                }
                AppButton {
                    visible: !!record.modelData.traceback && record.modelData.id !== undefined && !!view.actions
                             && !!view.actions.showCode
                    text: qsTr("Show code")
                    small: true
                    variant: "ghost"
                    onClicked: view.actions.showCode(record.modelData.id)
                }
            }
            Text {
                Layout.fillWidth: true
                text: record.modelData.message
                color: record.modelData.tone && record.modelData.tone !== "neutral" ? Theme.tone(record.modelData.tone) : Theme.text
                font.family: Theme.mono
                font.pixelSize: 12
                wrapMode: Text.WrapAnywhere
            }
            Text {
                visible: !!record.modelData.traceback
                Layout.fillWidth: true
                text: record.modelData.traceback
                      ? record.modelData.traceback.exception + (record.modelData.traceback.origin ? qsTr(" at %1").arg(record.modelData.traceback.origin) : "")
                      : ""
                color: Theme.text2
                font.family: Theme.mono
                font.pixelSize: 11
                wrapMode: Text.WrapAnywhere
            }
        }
    }
}
