import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Dialogs
import QtQuick.Layouts
import Bob.Backend

// Join (or start) the user's bob, connect to another running bob, or connect by hand.
Item {
    id: screen

    // the fields start from how the app starts bob; empty ones use the defaults shown as placeholders
    property string saveError
    property bool manual: false
    readonly property bool ready: (configField.text.trim() !== "" || Bob.defaultConfig !== "")
        && (binaryField.text.trim() !== "" || Bob.defaultBinary !== "")
    readonly property var connection: Desktop.parseConnection(connectionField.text, passwordField.text)
    readonly property bool looksLikeUrl: /^https?:\/\//i.test(connectionField.text.trim())
    readonly property bool manualReady: connection.url !== undefined && (!looksLikeUrl || passwordField.text.length > 0)

    function save(autoJoin) {
        saveError = Bob.saveSettings(configField.text, binaryField.text, autoJoin)
        return saveError === ""
    }


    Flickable {
        id: scroll
        anchors.fill: parent
        contentHeight: page.height
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar {}

        Item {
            id: page
            width: scroll.width
            height: Math.max(scroll.height, card.implicitHeight + 48)

            Rectangle {
                id: card
                anchors.centerIn: parent
                width: Math.min(560, parent.width - 48)
                implicitHeight: content.implicitHeight + 56
                radius: 14
                color: Theme.surface
                border.color: Theme.border

                ColumnLayout {
                    id: content
                    anchors.fill: parent
                    anchors.margins: 28
                    spacing: 0

                    RowLayout {
                        spacing: 10
                        Image {
                            source: "images/bob.png"
                            mipmap: true
                            fillMode: Image.PreserveAspectFit
                            Layout.preferredWidth: 26
                            Layout.preferredHeight: 26
                        }
                        Text {
                            text: "bob"
                            color: Theme.text
                            font.pixelSize: 22
                            font.bold: true
                        }
                    }
                    Text {
                        Layout.topMargin: 4
                        Layout.fillWidth: true
                        text: qsTr("Connect to the OpenCode server that bob runs in its sandbox.")
                        color: Theme.text2
                        wrapMode: Text.Wrap
                    }
                    Rectangle {
                        visible: Chat.connectError !== ""
                        Layout.topMargin: 14
                        Layout.fillWidth: true
                        implicitHeight: errorText.implicitHeight + 20
                        radius: Theme.radiusSmall
                        color: Theme.errorSoft
                        TextEdit {
                            id: errorText
                            anchors.fill: parent
                            anchors.margins: 10
                            readOnly: true
                            selectByMouse: true
                            text: Chat.connectError
                            color: Theme.error
                            font.family: Theme.mono
                            font.pixelSize: 12
                            wrapMode: TextEdit.Wrap
                        }
                    }

                    // ---------- your bob ----------
                    SectionHead {
                        text: qsTr("Your bob")
                    }
                    Text {
                        Layout.fillWidth: true
                        text: qsTr("One bob per user, shared by every app that uses it: this joins it, or starts it with your bob.toml when none runs. It stops 90 seconds after the last app leaves.")
                        color: Theme.text3
                        font.pixelSize: 13
                        wrapMode: Text.Wrap
                    }
                    RowLayout {
                        visible: Bob.state === "joined"
                        Layout.topMargin: 8
                        Layout.fillWidth: true
                        Text {
                            Layout.fillWidth: true
                            text: qsTr("This app holds <b>%1</b>.").arg(Bob.status.name ?? "")
                            textFormat: Text.StyledText
                            color: Theme.text
                            font.pixelSize: 13
                        }
                        AppButton {
                            text: qsTr("Leave it")
                            small: true
                            onClicked: Bob.leave()
                        }
                    }
                    FieldLabel {
                        text: qsTr("bob.toml")
                    }
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 8
                        AppTextField {
                            id: configField
                            Layout.fillWidth: true
                            text: Bob.config
                            placeholderText: Bob.defaultConfig || "/path/to/bob.toml"
                        }
                        AppButton {
                            text: qsTr("Choose…")
                            onClicked: configDialog.open()
                        }
                    }
                    FieldLabel {
                        text: qsTr("bob executable")
                    }
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 8
                        AppTextField {
                            id: binaryField
                            Layout.fillWidth: true
                            text: Bob.binary
                            placeholderText: Bob.defaultBinary || qsTr("bob-manager or bob on the PATH")
                        }
                        AppButton {
                            text: qsTr("Choose…")
                            onClicked: binaryDialog.open()
                        }
                    }
                    Text {
                        visible: screen.saveError !== ""
                        Layout.topMargin: 6
                        Layout.fillWidth: true
                        text: screen.saveError
                        textFormat: Text.PlainText
                        color: Theme.error
                        font.pixelSize: 13
                        wrapMode: Text.Wrap
                    }
                    RowLayout {
                        Layout.topMargin: 18
                        Layout.fillWidth: true
                        ToggleSwitch {
                            text: qsTr("Do this when the app opens")
                            checked: Bob.autoJoin
                            onToggled: screen.save(checked)
                        }
                        Item {
                            Layout.fillWidth: true
                        }
                        AppButton {
                            text: qsTr("Join or start bob")
                            variant: "primary"
                            enabled: screen.ready
                            onClicked: {
                                if (screen.save(Bob.autoJoin))
                                    Chat.joinBob()
                            }
                        }
                    }

                    // ---------- by hand ----------
                    AppButton {
                        visible: !screen.manual
                        Layout.topMargin: 12
                        text: qsTr("Connect by hand…")
                        variant: "ghost"
                        small: true
                        onClicked: screen.manual = true
                    }
                    ColumnLayout {
                        visible: screen.manual
                        Layout.fillWidth: true
                        spacing: 0
                        FieldLabel {
                            text: qsTr("Connection")
                        }
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: 90
                            radius: Theme.radiusSmall
                            color: Theme.surface
                            border.color: connectionField.activeFocus ? Theme.accent : Theme.border
                            ScrollView {
                                anchors.fill: parent
                                anchors.margins: 1
                                TextArea {
                                    id: connectionField
                                    wrapMode: TextEdit.WrapAnywhere
                                    placeholderText: '{"url":"http://127.0.0.1:52997","password":"…","name":"bob-…"}'
                                    placeholderTextColor: Theme.text3
                                    color: Theme.text
                                    font.family: Theme.mono
                                    font.pixelSize: 12
                                    background: null
                                }
                            }
                        }
                        FieldLabel {
                            visible: screen.looksLikeUrl
                            text: qsTr("Password")
                        }
                        AppTextField {
                            id: passwordField
                            visible: screen.looksLikeUrl
                            Layout.fillWidth: true
                            echoMode: TextInput.Password
                            onAccepted: if (screen.manualReady) Chat.connectTo(screen.connection)
                        }
                        RowLayout {
                            Layout.topMargin: 18
                            Layout.fillWidth: true
                            spacing: 12
                            Text {
                                Layout.fillWidth: true
                                text: qsTr("Paste a connection (JSON with url and password), or a URL and its password.")
                                color: Theme.text3
                                font.pixelSize: 13
                                wrapMode: Text.Wrap
                            }
                            AppButton {
                                text: qsTr("Connect")
                                variant: "primary"
                                enabled: screen.manualReady
                                onClicked: Chat.connectTo(screen.connection)
                            }
                        }
                    }
                }
            }
        }
    }

    FileDialog {
        id: configDialog
        title: qsTr("Choose a bob.toml")
        nameFilters: [qsTr("bob configuration (*.toml)")]
        onAccepted: configField.text = Desktop.localPath(selectedFile)
    }
    FileDialog {
        id: binaryDialog
        title: qsTr("Choose the bob executable")
        onAccepted: binaryField.text = Desktop.localPath(selectedFile)
    }

    component SectionHead: Text {
        Layout.topMargin: 22
        Layout.bottomMargin: 4
        Layout.fillWidth: true
        color: Theme.text
        font.pixelSize: 14
        font.weight: Font.DemiBold
    }
    component FieldLabel: Text {
        Layout.topMargin: 14
        Layout.bottomMargin: 6
        color: Theme.text2
        font.pixelSize: 13
        font.weight: Font.DemiBold
    }
}
