import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// A tool call as a row: status, name, summary and time; open, it shows the input and output, and
// a sub-agent's own work above them. A plugin serving the tool's MCP server may name the row and
// draw a card for its result instead of the raw call.
Rectangle {
    id: call

    required property string rowKey
    required property var item
    readonly property double elapsed: item.start > 0 && (item.end > 0 || item.running)
        ? (item.end > 0 ? item.end : Clock.now) - item.start : -1
    // the plugin's view of this call, if a plugin serves its server
    readonly property var view: {
        PluginRegistry.revision
        return PluginRegistry.enabled.length ? PluginRegistry.toolView(item) ?? {} : {}
    }
    readonly property bool rich: item.status === "completed" && !!view.body
    // a card opens by itself, until the user opens or closes the row
    readonly property bool expanded: item.expandedSet ? item.expanded : item.expanded || rich
    property bool raw: false
    readonly property string tone: view.tone ?? ""

    implicitHeight: column.implicitHeight
    radius: Theme.radius
    color: Theme.surface
    border.color: item.waiting ? Theme.warn : Theme.border

    ColumnLayout {
        id: column
        width: parent.width
        spacing: 0

        Item {
            Layout.fillWidth: true
            implicitHeight: Math.max(36, head.implicitHeight + 12)
            RowLayout {
                id: head
                anchors.fill: parent
                anchors.leftMargin: 10
                anchors.rightMargin: 10
                spacing: 8
                Item {
                    implicitWidth: 16
                    implicitHeight: 16
                    Icon {
                        visible: call.item.waiting
                        anchors.centerIn: parent
                        name: "shield"
                        size: 15
                        color: Theme.warn
                    }
                    Spinner {
                        visible: !call.item.waiting && call.item.running
                        anchors.centerIn: parent
                    }
                    Icon {
                        visible: !call.item.waiting && !call.item.running && (call.item.status === "completed" || call.item.status === "error")
                        anchors.centerIn: parent
                        readonly property bool failed: call.item.status === "error" && call.tone !== "ok"
                        name: failed ? "x" : "check"
                        size: 15
                        color: failed || call.tone === "error" ? Theme.error : call.tone === "warn" ? Theme.warn : Theme.ok
                    }
                    Dot {
                        visible: !call.item.waiting && !call.item.running && call.item.status !== "completed" && call.item.status !== "error"
                        anchors.centerIn: parent
                    }
                }
                Text {
                    text: call.view.label ?? call.item.label
                    textFormat: Text.PlainText
                    // the tool's own id, as OpenCode names it
                    HoverHandler {
                        id: labelHover
                    }
                    ToolTip.visible: labelHover.hovered && !!call.item.tool
                    ToolTip.text: call.item.tool ?? ""
                    ToolTip.delay: 800
                    color: Theme.text
                    font.pixelSize: 14
                    font.weight: Font.Medium
                }
                Text {
                    Layout.fillWidth: true
                    text: !call.item.waiting && call.view.summary !== undefined ? call.view.summary : call.item.summary
                    textFormat: Text.PlainText
                    color: call.item.status === "error" && !call.item.waiting && call.tone !== "ok" ? Theme.error : Theme.text2
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
                Text {
                    visible: call.elapsed >= 0
                    text: Desktop.formatDuration(Math.max(0, call.elapsed))
                    color: Theme.text3
                    font.pixelSize: 12
                }
                Icon {
                    name: "chevron"
                    size: 13
                    color: Theme.text3
                    rotation: call.expanded ? 90 : 0
                    Behavior on rotation { NumberAnimation { duration: 150 } }
                }
            }
            HoverHandler {
                cursorShape: Qt.PointingHandCursor
            }
            TapHandler {
                onTapped: Chat.setExpanded(call.rowKey, !call.expanded)
            }
        }

        // a sub-agent's own steps
        Rectangle {
            visible: call.expanded && call.item.child !== ""
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
        Loader {
            active: call.expanded && call.item.child !== ""
            visible: active
            Layout.fillWidth: true
            Layout.leftMargin: 12
            Layout.rightMargin: 12
            Layout.topMargin: 10
            Layout.bottomMargin: 12
            source: "SubAgentRun.qml"
            onLoaded: item.sessionId = Qt.binding(() => call.item.child)
        }

        Rectangle {
            visible: call.expanded
            Layout.fillWidth: true
            height: 1
            color: Theme.border
        }
        // the plugin's card for the result
        ColumnLayout {
            visible: call.expanded && call.rich && !call.raw
            Layout.fillWidth: true
            Layout.margins: 12
            spacing: 8
            Loader {
                id: card
                active: call.expanded && call.rich && !call.raw
                Layout.fillWidth: true
                sourceComponent: call.view.body ? call.view.body.component : null
                function apply() {
                    const properties = call.view.body ? call.view.body.properties || {} : {}
                    for (const key in properties)
                        item[key] = properties[key]
                }
                onLoaded: apply()
                Connections {
                    target: call
                    function onViewChanged() {
                        if (card.item)
                            card.apply()
                    }
                }
            }
            AppButton {
                text: qsTr("Show raw call")
                small: true
                variant: "ghost"
                onClicked: call.raw = true
            }
        }
        ColumnLayout {
            visible: call.expanded && (!call.rich || call.raw)
            Layout.fillWidth: true
            Layout.leftMargin: 12
            Layout.rightMargin: 12
            Layout.bottomMargin: 12
            spacing: 4
            AppButton {
                visible: call.rich
                Layout.topMargin: 8
                text: qsTr("Show card")
                small: true
                variant: "ghost"
                onClicked: call.raw = false
            }
            SectionLabel {
                text: qsTr("INPUT")
            }
            Pre {
                Layout.fillWidth: true
                text: call.item.input
            }
            SectionLabel {
                visible: call.item.output !== ""
                text: call.item.outputLabel.toUpperCase()
            }
            Pre {
                visible: call.item.output !== ""
                Layout.fillWidth: true
                text: call.item.output
            }
        }
    }

    component SectionLabel: Text {
        Layout.topMargin: 6
        color: Theme.text3
        font.pixelSize: 11
        font.weight: Font.DemiBold
        font.letterSpacing: 0.4
    }
    component Pre: Rectangle {
        property alias text: preText.text
        implicitHeight: Math.min(320, preText.implicitHeight + 16)
        radius: Theme.radiusSmall
        color: Theme.surface2
        clip: true
        Flickable {
            anchors.fill: parent
            anchors.margins: 8
            contentHeight: preText.implicitHeight
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            TextEdit {
                id: preText
                width: parent.width
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.WrapAtWordBoundaryOrAnywhere
                textFormat: TextEdit.PlainText
                color: Theme.text
                selectionColor: Theme.accentSoft
                selectedTextColor: Theme.text
                font.family: Theme.mono
                font.pixelSize: 12
            }
        }
    }
}
