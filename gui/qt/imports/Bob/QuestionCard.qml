import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// OpenCode's question to the user: pick from its options, or type an answer.
RequestCard {
    id: card

    required property var request
    readonly property var questions: request.questions ?? []
    // chosen labels and typed answers, per question
    property var answers: questions.map(() => [])
    property var custom: questions.map(() => "")
    readonly property bool complete: questions.every((_, i) => answers[i].length > 0 || custom[i].trim() !== "")

    function choose(qi, label, multiple) {
        const next = answers.slice()
        const list = next[qi].slice()
        const at = list.indexOf(label)
        if (multiple)
            at === -1 ? list.push(label) : list.splice(at, 1)
        next[qi] = multiple ? list : [label]
        answers = next
    }

    Repeater {
        model: card.questions
        delegate: ColumnLayout {
            id: question
            required property var modelData
            required property int index
            Layout.fillWidth: true
            Layout.bottomMargin: 6
            spacing: 6
            RowLayout {
                spacing: 8
                Icon {
                    name: "question"
                    color: Theme.info
                }
                Text {
                    Layout.fillWidth: true
                    text: question.modelData.question
                    textFormat: Text.PlainText
                    color: Theme.text
                    font.pixelSize: 14
                    font.weight: Font.DemiBold
                    wrapMode: Text.Wrap
                }
            }
            Repeater {
                model: question.modelData.options ?? []
                delegate: Rectangle {
                    id: option
                    required property var modelData
                    readonly property bool selected: card.answers[question.index].indexOf(modelData.label) !== -1
                    Layout.fillWidth: true
                    implicitHeight: optionRow.implicitHeight + 14
                    radius: Theme.radiusSmall
                    color: selected ? Theme.accentSoft : "transparent"
                    border.color: selected ? Theme.accent : Theme.border
                    RowLayout {
                        id: optionRow
                        x: 10
                        y: 7
                        width: parent.width - 20
                        spacing: 8
                        Rectangle {
                            Layout.alignment: Qt.AlignTop
                            Layout.topMargin: 2
                            implicitWidth: 14
                            implicitHeight: 14
                            radius: question.modelData.multiple ? 3 : 7
                            color: option.selected ? Theme.accent : Theme.surface
                            border.color: option.selected ? Theme.accent : Theme.borderStrong
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 0
                            Text {
                                Layout.fillWidth: true
                                text: option.modelData.label
                                textFormat: Text.PlainText
                                color: Theme.text
                                wrapMode: Text.Wrap
                            }
                            Text {
                                visible: !!option.modelData.description
                                Layout.fillWidth: true
                                text: option.modelData.description ?? ""
                                textFormat: Text.PlainText
                                color: Theme.text3
                                font.pixelSize: 12
                                wrapMode: Text.Wrap
                            }
                        }
                    }
                    TapHandler {
                        onTapped: card.choose(question.index, option.modelData.label, !!question.modelData.multiple)
                    }
                    HoverHandler {
                        cursorShape: Qt.PointingHandCursor
                    }
                }
            }
            AppTextField {
                visible: question.modelData.custom !== false
                Layout.fillWidth: true
                placeholderText: qsTr("Or type your own answer")
                onTextChanged: {
                    const next = card.custom.slice()
                    next[question.index] = text
                    card.custom = next
                }
            }
        }
    }
    RowLayout {
        spacing: 8
        AppButton {
            text: qsTr("Answer")
            variant: "primary"
            enabled: card.complete
            onClicked: Chat.replyQuestion(card.request.id,
                card.answers.map((list, i) => card.custom[i].trim() ? list.concat([card.custom[i].trim()]) : list))
        }
        AppButton {
            text: qsTr("Dismiss")
            onClicked: Chat.rejectQuestion(card.request.id)
        }
    }
}
