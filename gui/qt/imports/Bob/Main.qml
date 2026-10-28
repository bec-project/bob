import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts
import Bob.Backend

// The app's one window: the connect screen until a bob is connected, then the chat.
ApplicationWindow {
    id: window

    width: 1280
    height: 840
    minimumWidth: 720
    minimumHeight: 480
    visible: true
    title: "bob"
    color: Theme.bg
    font.pixelSize: Theme.fontSize

    palette {
        window: Theme.bg
        windowText: Theme.text
        base: Theme.surface
        alternateBase: Theme.surface2
        text: Theme.text
        button: Theme.surface
        buttonText: Theme.text
        brightText: "#ffffff"
        highlight: Theme.accent
        highlightedText: "#ffffff"
        placeholderText: Theme.text3
        toolTipBase: Theme.surface
        toolTipText: Theme.text
        light: Theme.surface2
        midlight: Theme.surface3
        mid: Theme.border
        dark: Theme.borderStrong
        shadow: Theme.shadow
        link: Theme.accentText
    }

    // On macOS Qt reads Ctrl as ⌘; Meta, the Control key there, works as well.
    readonly property bool mac: Qt.platform.os === "osx"
    Shortcut {
        sequences: window.mac ? ["Ctrl+Shift+O", "Meta+Shift+O"] : ["Ctrl+Shift+O"]
        enabled: Chat.phase === "ready"
        onActivated: Chat.selectSession("")
    }
    Shortcut {
        sequences: window.mac ? ["Ctrl+,", "Meta+,"] : ["Ctrl+,"]
        enabled: Chat.phase === "ready"
        onActivated: settings.open()
    }

    Loader {
        anchors.fill: parent
        sourceComponent: {
            switch (Chat.phase) {
            case "ready": return chat
            case "starting": return joining
            case "connecting": return connecting
            default: return connect
            }
        }
    }

    Component {
        id: chat
        ChatView {
            onSettingsRequested: settings.open()
        }
    }
    Component {
        id: connect
        ConnectScreen {}
    }
    Component {
        id: joining
        Working {
            text: Bob.state === "joining"
                ? qsTr("Joining your bob, or starting it with %1…").arg(Desktop.fileName(Bob.status.config))
                : qsTr("Joining your bob…")
            AppButton {
                text: qsTr("Cancel")
                small: true
                variant: "ghost"
                onClicked: Chat.leaveBob()
            }
        }
    }
    Component {
        id: connecting
        Working {
            text: qsTr("Connecting…")
        }
    }

    SettingsDialog {
        id: settings
        objectName: "settings"
    }
    RestartDialog {}
}
