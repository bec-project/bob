import QtQuick
import QtQuick.Layouts
import Bob

// The instance's settings: a bearer token for a server started with --token-file.
ColumnLayout {
    property var context

    spacing: 6

    Text {
        Layout.fillWidth: true
        text: qsTr("Bearer token, if the server needs one (kept for this window only)")
        color: Theme.text3
        font.pixelSize: 12
        wrapMode: Text.Wrap
    }
    AppTextField {
        Layout.fillWidth: true
        implicitHeight: 30
        echoMode: TextInput.Password
        text: context && context.server ? context.server.token : ""
        onTextEdited: if (context && context.server) context.server.token = text
    }
}
