import QtQuick
import QtQuick.Layouts
import Bob

// Problems in BEC's log, grouped into incidents (bec_log_analysis): what happened, its likely
// cause, what to try, where it was raised, and the records around it.
ColumnLayout {
    id: view

    property var incidents: []
    property var actions

    spacing: 8

    Text {
        visible: view.incidents.length === 0
        text: qsTr("No incidents.")
        color: Theme.text3
    }
    Repeater {
        model: view.incidents
        delegate: Rectangle {
            id: incident
            required property var modelData
            property bool open: false
            readonly property string when: {
                const first = modelData.first.slice(11, 19)
                return modelData.first === modelData.last || !modelData.last ? first : first + "–" + modelData.last.slice(11, 19)
            }
            Layout.fillWidth: true
            implicitHeight: column.implicitHeight + 16
            radius: Theme.radiusSmall
            color: Theme.surface
            border.color: Theme.border
            Rectangle {
                width: 3
                height: parent.height
                radius: 1
                color: Theme.tone(incident.modelData.tone)
            }
            ColumnLayout {
                id: column
                x: 12
                y: 8
                width: parent.width - 22
                spacing: 4
                RowLayout {
                    spacing: 6
                    Text {
                        text: incident.modelData.ref
                        color: Theme.text3
                        font.family: Theme.mono
                        font.pixelSize: 12
                    }
                    Text {
                        Layout.fillWidth: true
                        text: incident.modelData.title
                        color: Theme.text
                        font.weight: Font.DemiBold
                        font.pixelSize: 13
                        elide: Text.ElideRight
                    }
                    Chip {
                        visible: incident.modelData.count > 1
                        text: "×" + incident.modelData.count
                    }
                    Text {
                        text: incident.when
                        color: Theme.text3
                        font.pixelSize: 12
                    }
                }
                Text {
                    Layout.fillWidth: true
                    text: incident.modelData.message
                    color: Theme.text2
                    font.family: Theme.mono
                    font.pixelSize: 12
                    wrapMode: Text.WrapAnywhere
                    maximumLineCount: 6
                    elide: Text.ElideRight
                }
                Text {
                    visible: !!incident.modelData.origin
                    Layout.fillWidth: true
                    text: incident.modelData.origin
                          ? qsTr("raised at <code>%1</code>").arg(incident.modelData.origin.where)
                            + (incident.modelData.origin.code ? "<br><code>" + incident.modelData.origin.code + "</code>" : "")
                          : ""
                    textFormat: Text.StyledText
                    color: Theme.text3
                    font.pixelSize: 12
                    wrapMode: Text.WrapAnywhere
                }
                Text {
                    visible: incident.modelData.likelyCause !== ""
                    Layout.fillWidth: true
                    text: qsTr("<b>Likely cause</b> %1").arg(incident.modelData.likelyCause)
                    textFormat: Text.StyledText
                    color: Theme.text
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: incident.modelData.suggestion !== ""
                    Layout.fillWidth: true
                    text: qsTr("<b>Try</b> %1").arg(incident.modelData.suggestion)
                    textFormat: Text.StyledText
                    color: Theme.text
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                Flow {
                    Layout.fillWidth: true
                    spacing: 6
                    Repeater {
                        model: incident.modelData.services
                        delegate: Chip { required property string modelData; text: modelData }
                    }
                    Repeater {
                        model: incident.modelData.devices
                        delegate: Chip { required property string modelData; text: modelData; tone: "info" }
                    }
                    Chip {
                        visible: incident.modelData.scan !== ""
                        text: qsTr("scan %1").arg(incident.modelData.scan)
                    }
                    Text {
                        visible: incident.modelData.related.length > 0
                        text: qsTr("with %1").arg(incident.modelData.related.join(", "))
                        color: Theme.text3
                        font.pixelSize: 12
                    }
                }
                Flow {
                    Layout.fillWidth: true
                    spacing: 6
                    AppButton {
                        visible: !!view.actions && !!view.actions.ask
                        text: qsTr("Ask about it")
                        small: true
                        onClicked: view.actions.ask(qsTr("Explain BEC log incident %1 (%2, first record %3) and suggest a fix.")
                            .arg(incident.modelData.ref).arg(incident.modelData.title)
                            .arg(incident.modelData.firstId !== undefined ? incident.modelData.firstId : "?"))
                    }
                    AppButton {
                        visible: incident.modelData.tracebackRecord !== undefined && !!view.actions && !!view.actions.showCode
                        text: qsTr("Show code")
                        small: true
                        onClicked: view.actions.showCode(incident.modelData.tracebackRecord)
                    }
                    AppButton {
                        visible: incident.modelData.firstId !== undefined && !!view.actions && !!view.actions.showRecords
                        text: qsTr("Show records")
                        small: true
                        onClicked: view.actions.showRecords(incident.modelData.firstId)
                    }
                    AppButton {
                        visible: incident.modelData.context.length > 0
                        text: incident.open ? qsTr("Hide") : qsTr("Records around it")
                        iconName: "chevron"
                        small: true
                        variant: "ghost"
                        onClicked: incident.open = !incident.open
                    }
                }
                BecLogRecords {
                    visible: incident.open
                    Layout.fillWidth: true
                    records: incident.modelData.context
                    actions: view.actions
                }
            }
        }
    }
}
