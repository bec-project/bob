import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// Totals for the open chat: messages, time answering, tokens, the context of the last model call
// against the model's limit, tool calls and cost; and Compact.
Rectangle {
    id: bar

    readonly property var usage: Chat.usage

    // a total, stronger than its label
    function b(value) {
        return "<font color=\"" + Theme.text2 + "\"><b>" + value + "</b></font>"
    }

    visible: usage.turns > 0
    implicitHeight: 32
    color: Theme.surface
    clip: true

    Rectangle {
        anchors.bottom: parent.bottom
        width: parent.width
        height: 1
        color: Theme.border
    }

    // scrolls sideways when the window is too narrow for every total
    Flickable {
        id: flick
        anchors.fill: parent
        anchors.leftMargin: 16
        anchors.rightMargin: 16
        contentWidth: stats.width
        flickableDirection: Flickable.HorizontalFlick
        boundsBehavior: Flickable.StopAtBounds
        clip: true

        RowLayout {
            id: stats
            width: Math.max(flick.width, implicitWidth)
            height: flick.height
            spacing: 12

            Stat {
                text: qsTr("%1 %2").arg(bar.b(bar.usage.turns)).arg(bar.usage.turns === 1 ? qsTr("message") : qsTr("messages"))
            }
            Separator {}
            Stat {
                text: qsTr("%1 answering").arg(bar.b(bar.usage.time))
            }
            Separator {}
            Stat {
                text: qsTr("in %1").arg(bar.b(bar.usage.input))
                tip: qsTr("Input tokens sent to the model, summed over every step")
            }
            Stat {
                text: qsTr("out %1").arg(bar.b(bar.usage.output))
            }
            Stat {
                visible: !!bar.usage.reasoning
                text: qsTr("thinking %1").arg(bar.b(bar.usage.reasoning))
            }
            Stat {
                visible: !!bar.usage.cached
                text: qsTr("cached %1").arg(bar.b(bar.usage.cached))
            }
            Separator {}
            RowLayout {
                spacing: 6
                HoverHandler {
                    id: meterHover
                }
                ToolTip.visible: meterHover.hovered
                ToolTip.delay: 500
                ToolTip.text: bar.usage.limitTokens > 0
                    ? qsTr("The last model call used %1 of %2 context tokens. OpenCode compacts the chat on its own when it fills up.")
                        .arg(bar.usage.contextTokens).arg(bar.usage.limitTokens)
                    : qsTr("Tokens in the context of the last model call. Set provider.context_limit in bob.toml to see how full it is.")
                Stat {
                    text: bar.usage.limitTokens > 0 ? qsTr("context %1 / %2").arg(bar.b(bar.usage.context)).arg(bar.usage.limit)
                                                     : qsTr("context %1").arg(bar.b(bar.usage.context))
                }
                Rectangle {
                    visible: bar.usage.limitTokens > 0
                    implicitWidth: 60
                    implicitHeight: 6
                    radius: 3
                    color: Theme.surface3
                    Rectangle {
                        width: parent.width * (bar.usage.fill ?? 0)
                        height: parent.height
                        radius: 3
                        color: (bar.usage.fill ?? 0) >= 0.8 ? Theme.warn : Theme.accent
                    }
                }
                Stat {
                    visible: bar.usage.limitTokens > 0
                    text: Math.round((bar.usage.fill ?? 0) * 100) + "%"
                }
            }
            Separator {
                visible: bar.usage.tools > 0
            }
            Stat {
                visible: bar.usage.tools > 0
                text: qsTr("%1 %2").arg(bar.b(bar.usage.tools)).arg(bar.usage.tools === 1 ? qsTr("tool call") : qsTr("tool calls"))
            }
            Separator {
                visible: !!bar.usage.cost
            }
            Stat {
                visible: !!bar.usage.cost
                text: bar.b(bar.usage.cost)
            }
            Item {
                Layout.fillWidth: true
            }
            AppButton {
                text: qsTr("Compact")
                iconName: "compress"
                small: true
                variant: "ghost"
                enabled: !Chat.busy && !Chat.compacting
                tip: qsTr("Summarise the chat so far; later messages start from the summary")
                onClicked: Chat.compact()
            }
        }
    }

    component Stat: Text {
        property string tip
        color: Theme.text3
        font.pixelSize: 12
        textFormat: Text.StyledText
        HoverHandler {
            id: statHover
        }
        ToolTip.visible: statHover.hovered
        ToolTip.text: tip || qsTr("Totals for this chat")
        ToolTip.delay: 500
    }
    component Separator: Rectangle {
        implicitWidth: 1
        implicitHeight: 12
        color: Theme.border
    }
}
