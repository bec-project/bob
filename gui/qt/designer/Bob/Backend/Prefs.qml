pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Prefs (src/gui/prefs.h).
QtObject {
    property string theme: "system"
    property string model: ""
    property bool thinking: true
    property string agent: ""
    property bool sidebar: true
    property string panel: ""
    property var widths: ({})

    function setWidth(key, width) {
        const next = Object.assign({}, widths)
        if (width < 0)
            delete next[key]
        else
            next[key] = width
        widths = next
    }
}
