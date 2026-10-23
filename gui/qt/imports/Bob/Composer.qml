import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// Where the user writes. Enter sends, Shift+Enter breaks the line, Up and Down on the first or
// last line step through earlier prompts like a shell, Esc stops the answer.
Item {
    id: composer

    readonly property bool ready: Chat.stream === "open" || Chat.stream === "retrying"
    // set while the app, not the user, changes the text
    property bool setting: false

    function setText(text) {
        setting = true
        input.text = text
        input.cursorPosition = input.length
        setting = false
    }
    function submit() {
        const value = input.text.trim()
        if (!value || !ready)
            return
        setText("")
        Chat.send(value, true)
    }
    function step(up) {
        if (input.selectedText !== "")
            return false
        // only from the first (Up) or last (Down) line as shown, wrapped lines included
        const caret = input.positionToRectangle(input.cursorPosition).y
        const edge = input.positionToRectangle(up ? 0 : input.length).y
        if (up ? caret > edge : caret < edge)
            return false
        const next = Chat.recall(up, input.text)
        if (next.text === undefined)
            return false
        setText(next.text)
        return true
    }

    implicitHeight: column.implicitHeight + 22

    Connections {
        target: Chat
        function onComposerRequest(text, replace) {
            composer.setText(replace || !input.text.trim() ? text : input.text.trimEnd() + "\n" + text)
            input.forceActiveFocus()
        }
    }
    readonly property string sessionId: Chat.currentId
    onSessionIdChanged: input.forceActiveFocus()
    Component.onCompleted: input.forceActiveFocus()

    ColumnLayout {
        id: column
        x: Math.round((parent.width - width) / 2)
        y: 8
        width: Math.min(Theme.column, parent.width - 40)
        spacing: 6

        Rectangle {
            visible: Chat.notice.text !== undefined
            Layout.fillWidth: true
            Layout.bottomMargin: 2
            implicitHeight: noticeRow.implicitHeight + 16
            radius: Theme.radiusSmall
            color: Chat.notice.tone === "info" ? Theme.infoSoft : Theme.errorSoft
            RowLayout {
                id: noticeRow
                anchors.fill: parent
                anchors.leftMargin: 12
                anchors.rightMargin: 4
                TextEdit {
                    Layout.fillWidth: true
                    text: Chat.notice.text ?? ""
                    readOnly: true
                    selectByMouse: true
                    wrapMode: TextEdit.Wrap
                    color: Chat.notice.tone === "info" ? Theme.info : Theme.error
                    font.pixelSize: 13
                }
                IconButton {
                    iconName: "x"
                    iconSize: 14
                    tip: qsTr("Dismiss")
                    onClicked: Chat.dismissNotice()
                }
            }
        }

        Rectangle {
            Layout.fillWidth: true
            implicitHeight: frame.implicitHeight
            radius: 14
            color: Theme.surface
            border.color: input.activeFocus ? Theme.accent : Theme.borderStrong

            ColumnLayout {
                id: frame
                width: parent.width
                spacing: 0

                ScrollView {
                    id: scroll
                    Layout.fillWidth: true
                    Layout.preferredHeight: Math.min(260, Math.max(48, input.implicitHeight))
                    TextArea {
                        id: input
                        topPadding: 12
                        leftPadding: 14
                        rightPadding: 14
                        bottomPadding: 4
                        wrapMode: TextEdit.Wrap
                        color: Theme.text
                        selectionColor: Theme.accentSoft
                        selectedTextColor: Theme.text
                        placeholderText: Chat.busy ? qsTr("Queue a follow-up…") : qsTr("Ask anything")
                        placeholderTextColor: Theme.text3
                        font.pixelSize: 14
                        background: null
                        onTextChanged: if (!composer.setting) Chat.endRecall()
                        Keys.onPressed: (event) => {
                            const plain = !(event.modifiers & (Qt.ShiftModifier | Qt.AltModifier | Qt.ControlModifier | Qt.MetaModifier))
                            if ((event.key === Qt.Key_Up || event.key === Qt.Key_Down) && plain && !inputMethodComposing) {
                                event.accepted = composer.step(event.key === Qt.Key_Up)
                            } else if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter)
                                       && !(event.modifiers & Qt.ShiftModifier) && !inputMethodComposing) {
                                event.accepted = true
                                composer.submit()
                            } else if (event.key === Qt.Key_Escape && Chat.busy) {
                                event.accepted = true
                                Chat.abort()
                            }
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    Layout.leftMargin: 12
                    Layout.rightMargin: 8
                    Layout.bottomMargin: 8
                    Layout.topMargin: 6
                    spacing: 8
                    Text {
                        Layout.fillWidth: true
                        text: composer.ready ? "" : qsTr("Waiting for the server…")
                        color: Theme.text3
                        font.pixelSize: 12
                    }
                    AppButton {
                        visible: Chat.busy
                        text: qsTr("Stop")
                        iconName: "stop"
                        small: true
                        tip: qsTr("Stop (Esc)")
                        onClicked: Chat.abort()
                    }
                    AppButton {
                        text: qsTr("Send")
                        iconName: "send"
                        small: true
                        variant: "primary"
                        enabled: input.text.trim() !== "" && composer.ready
                        tip: qsTr("Send (Enter)")
                        onClicked: composer.submit()
                    }
                }
            }
        }

        Row {
            Layout.alignment: Qt.AlignHCenter
            spacing: 4
            Kbd { text: "Enter" }
            Hint { text: qsTr("send ·") }
            Kbd { text: "Shift" }
            Hint { text: "+" }
            Kbd { text: "Enter" }
            Hint { text: qsTr("new line ·") }
            Kbd { text: "↑" }
            Hint { text: qsTr("earlier prompts ·") }
            Kbd { text: "Esc" }
            Hint { text: qsTr("stop") }
        }
    }

    component Hint: Text {
        color: Theme.text3
        font.pixelSize: 11
        anchors.verticalCenter: parent ? parent.verticalCenter : undefined
    }
}
