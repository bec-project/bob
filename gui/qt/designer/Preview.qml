import QtQuick
import Bob

// What Design Studio shows: the chat window, with the stand-ins in Bob/Backend for the app's C++.
Rectangle {
    width: 1280
    height: 840
    color: Theme.bg

    ChatView {
        anchors.fill: parent
    }
}
