pragma Singleton

import QtQuick
import Bob.Backend

// The time the views show running timers with: it ticks while something in the open chat runs,
// and every half minute for the relative times in the chat list.
QtObject {
    property double now: Date.now()

    property Timer fast: Timer {
        interval: 200
        repeat: true
        running: Chat.running
        onTriggered: Clock.now = Date.now()
        onRunningChanged: Clock.now = Date.now()
    }
    property Timer slow: Timer {
        interval: 30000
        repeat: true
        running: true
        onTriggered: Clock.now = Date.now()
    }
}
