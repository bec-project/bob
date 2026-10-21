import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob
import Bob.Backend

// One BEC report card (cards.js); sections without content are left out. `actions` gives buttons
// on findings, incidents and log records ({ask, showCode, showRecords}); without it they stay out.
ColumnLayout {
    id: view

    property var card: ({ badges: [], stats: [], findings: [], links: [], lines: [] })
    property bool compact: false
    property var actions

    function toneColor(tone) {
        return tone === "accent" ? Theme.accentText : tone && tone !== "neutral" ? Theme.tone(tone) : Theme.text
    }

    spacing: 10

    Flow {
        visible: !view.compact || view.card.badges.length > 0
        Layout.fillWidth: true
        spacing: 8
        Text {
            visible: !view.compact
            text: view.card.title ?? ""
            color: Theme.text
            font.weight: Font.DemiBold
        }
        Repeater {
            model: view.card.badges
            delegate: Chip {
                required property var modelData
                text: modelData.text
                tone: modelData.tone === "neutral" ? "" : modelData.tone
            }
        }
    }
    Text {
        visible: !view.compact && !!view.card.summary
        Layout.fillWidth: true
        text: view.card.summary ?? ""
        color: Theme.text2
        wrapMode: Text.Wrap
    }

    // a reading against its limits
    Rectangle {
        visible: !!view.card.gauge
        Layout.fillWidth: true
        implicitHeight: gauge.implicitHeight + 20
        radius: Theme.radiusSmall
        color: Theme.surface2
        ColumnLayout {
            id: gauge
            x: 12
            y: 10
            width: parent.width - 24
            spacing: 4
            readonly property var info: view.card.gauge ?? {}
            RowLayout {
                spacing: 10
                Text {
                    text: gauge.info.value ?? ""
                    color: Theme.text
                    font.pixelSize: 20
                    font.weight: Font.DemiBold
                }
                Text {
                    Layout.fillWidth: true
                    text: gauge.info.label ?? ""
                    color: Theme.text3
                }
                Text {
                    text: gauge.info.age ?? ""
                    color: Theme.text3
                    font.pixelSize: 12
                }
            }
            Item {
                visible: (gauge.info.fraction ?? -1) >= 0
                Layout.fillWidth: true
                Layout.topMargin: 6
                implicitHeight: 14
                Rectangle {
                    anchors.verticalCenter: parent.verticalCenter
                    width: parent.width
                    height: 6
                    radius: 3
                    color: Theme.surface3
                }
                Rectangle {
                    x: Math.max(0, parent.width * (gauge.info.fraction ?? 0) - 2)
                    width: 4
                    height: 14
                    radius: 2
                    color: gauge.info.tone === "warn" ? Theme.warn : Theme.accent
                }
            }
            RowLayout {
                visible: (gauge.info.fraction ?? -1) >= 0
                Layout.fillWidth: true
                Text { text: gauge.info.low ?? ""; color: Theme.text3; font.pixelSize: 11 }
                Item { Layout.fillWidth: true }
                Text { text: qsTr("limits"); color: Theme.text3; font.pixelSize: 11 }
                Item { Layout.fillWidth: true }
                Text { text: gauge.info.high ?? ""; color: Theme.text3; font.pixelSize: 11 }
            }
        }
    }

    Flow {
        visible: view.card.stats.length > 0
        Layout.fillWidth: true
        spacing: 8
        Repeater {
            model: view.card.stats
            delegate: Rectangle {
                required property var modelData
                implicitWidth: Math.max(72, statColumn.implicitWidth + 20)
                implicitHeight: statColumn.implicitHeight + 12
                radius: Theme.radiusSmall
                color: Theme.surface2
                Column {
                    id: statColumn
                    x: 10
                    y: 6
                    Text {
                        text: modelData.value
                        color: view.toneColor(modelData.tone)
                        font.pixelSize: 15
                        font.weight: Font.DemiBold
                    }
                    Text {
                        text: modelData.label
                        color: Theme.text3
                        font.pixelSize: 11
                    }
                }
            }
        }
    }

    Repeater {
        model: view.card.findings
        delegate: Rectangle {
            id: finding
            required property var modelData
            Layout.fillWidth: true
            implicitHeight: findingColumn.implicitHeight + 14
            radius: 4
            color: Theme.surface2
            Rectangle {
                width: 3
                height: parent.height
                color: Theme.tone(finding.modelData.tone)
            }
            ColumnLayout {
                id: findingColumn
                x: 12
                y: 7
                width: parent.width - 22
                spacing: 2
                Text {
                    Layout.fillWidth: true
                    text: finding.modelData.text
                    color: Theme.text
                    font.pixelSize: 13
                    wrapMode: Text.Wrap
                }
                RowLayout {
                    visible: finding.modelData.next !== ""
                    spacing: 6
                    Text {
                        text: qsTr("Next: <code>%1</code>").arg(finding.modelData.next)
                        textFormat: Text.StyledText
                        color: Theme.text3
                        font.pixelSize: 12
                    }
                    AppButton {
                        visible: !!view.actions && !!view.actions.ask
                        text: qsTr("Ask")
                        small: true
                        variant: "ghost"
                        onClicked: view.actions.ask(qsTr("Run %1 and tell me what it shows.").arg(finding.modelData.next))
                    }
                }
            }
        }
    }

    BecIncidents {
        visible: !!view.card.incidents
        Layout.fillWidth: true
        incidents: view.card.incidents ?? []
        actions: view.actions
    }
    BecLogRecords {
        visible: !!view.card.records
        Layout.fillWidth: true
        records: view.card.records ?? []
        actions: view.actions
    }
    BecTraceback {
        visible: !!view.card.frames
        Layout.fillWidth: true
        frames: view.card.frames ?? []
        exceptions: view.card.exceptions ?? []
    }

    // a table, scrolling sideways when it is wider than the card
    Rectangle {
        visible: !!view.card.table && view.card.table.rows.length > 0
        Layout.fillWidth: true
        implicitHeight: Math.min(340, table.implicitHeight) + (note.visible ? note.implicitHeight : 0) + 2
        radius: Theme.radiusSmall
        color: "transparent"
        border.color: Theme.border
        clip: true
        Flickable {
            id: tableView
            x: 1
            y: 1
            width: parent.width - 2
            height: Math.min(340, table.implicitHeight)
            contentWidth: Math.max(width, table.implicitWidth)
            contentHeight: table.implicitHeight
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.horizontal: ScrollBar {}
            ScrollBar.vertical: ScrollBar {}
            GridLayout {
                id: table
                readonly property var content: view.card.table ?? { columns: [], rows: [] }
                width: tableView.contentWidth
                columns: Math.max(1, content.columns.length)
                columnSpacing: 0
                rowSpacing: 0
                Repeater {
                    model: table.content.columns
                    delegate: Rectangle {
                        required property var modelData
                        Layout.fillWidth: true
                        implicitWidth: headText.implicitWidth + 20
                        implicitHeight: headText.implicitHeight + 12
                        color: Theme.surface2
                        Text {
                            id: headText
                            x: 10
                            anchors.verticalCenter: parent.verticalCenter
                            text: modelData
                            color: Theme.text3
                            font.pixelSize: 12
                            font.weight: Font.DemiBold
                        }
                    }
                }
                Repeater {
                    model: {
                        // the cells in reading order
                        const cells = []
                        for (const row of table.content.rows)
                            for (const cell of row)
                                cells.push(cell)
                        return cells
                    }
                    delegate: Rectangle {
                        required property var modelData
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        Layout.maximumWidth: 420
                        implicitWidth: Math.min(420, cellText.implicitWidth + 20)
                        implicitHeight: cellText.implicitHeight + 10
                        color: "transparent"
                        Rectangle {
                            width: parent.width
                            height: 1
                            color: Theme.border
                        }
                        Text {
                            id: cellText
                            x: 10
                            y: 5
                            width: Math.min(400, implicitWidth)
                            text: modelData.text
                            color: view.toneColor(modelData.tone)
                            font.pixelSize: 12
                            wrapMode: Text.Wrap
                        }
                    }
                }
            }
        }
        Text {
            id: note
            visible: !!view.card.table && !!view.card.table.note
            anchors.bottom: parent.bottom
            width: parent.width
            leftPadding: 10
            topPadding: 5
            bottomPadding: 5
            text: view.card.table ? view.card.table.note : ""
            color: Theme.text3
            font.pixelSize: 12
        }
    }

    // log lines (bec_logs)
    Rectangle {
        visible: view.card.lines.length > 0
        Layout.fillWidth: true
        implicitHeight: Math.min(300, lines.implicitHeight + 2)
        radius: Theme.radiusSmall
        color: "transparent"
        border.color: Theme.border
        clip: true
        Flickable {
            anchors.fill: parent
            anchors.margins: 1
            contentHeight: lines.implicitHeight
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            Column {
                id: lines
                width: parent.width
                Repeater {
                    model: view.card.lines
                    delegate: Column {
                        required property var modelData
                        required property int index
                        width: lines.width
                        topPadding: 4
                        bottomPadding: 4
                        leftPadding: 10
                        rightPadding: 10
                        Text {
                            width: parent.width - 20
                            text: modelData.text
                            color: view.toneColor(modelData.tone)
                            font.family: Theme.mono
                            font.pixelSize: 12
                            wrapMode: Text.WrapAnywhere
                        }
                        Text {
                            text: modelData.meta
                            color: Theme.text3
                            font.family: Theme.mono
                            font.pixelSize: 11
                        }
                    }
                }
            }
        }
    }

    // code, with a copy button
    ColumnLayout {
        visible: !!view.card.code
        Layout.fillWidth: true
        spacing: 4
        readonly property var code: view.card.code ?? { label: "", text: "" }
        Text {
            text: parent.code.label
            color: Theme.text3
            font.pixelSize: 11
        }
        Item {
            Layout.fillWidth: true
            implicitHeight: codeLines.visible ? codeLines.implicitHeight : codeBox.implicitHeight
            BecCodeLines {
                id: codeLines
                visible: /^\s*>?\s*\d+\|/.test(parent.parent.code.text)
                width: parent.width
                code: parent.parent.code.text
            }
            CodeBlock {
                id: codeBox
                visible: !codeLines.visible
                width: parent.width
                text: parent.parent.code.text
            }
        }
    }

    Repeater {
        model: view.card.links
        delegate: Rectangle {
            id: link
            required property var modelData
            Layout.fillWidth: true
            implicitHeight: linkColumn.implicitHeight + 16
            radius: Theme.radiusSmall
            color: "transparent"
            border.color: linkHover.hovered && modelData.url ? Theme.accent : Theme.border
            ColumnLayout {
                id: linkColumn
                x: 10
                y: 8
                width: parent.width - 20
                spacing: 2
                Text {
                    Layout.fillWidth: true
                    text: link.modelData.title
                    color: Theme.accentText
                    font.weight: Font.Medium
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: !!link.modelData.subtitle
                    Layout.fillWidth: true
                    text: link.modelData.subtitle
                    color: Theme.text3
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
                Text {
                    visible: !!link.modelData.snippet
                    Layout.fillWidth: true
                    text: link.modelData.snippet
                    color: Theme.text3
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
            }
            HoverHandler {
                id: linkHover
                cursorShape: link.modelData.url ? Qt.PointingHandCursor : Qt.ArrowCursor
            }
            TapHandler {
                enabled: !!link.modelData.url
                onTapped: Desktop.openExternal(link.modelData.url)
            }
        }
    }
}
