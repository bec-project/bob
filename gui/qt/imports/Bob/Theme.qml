pragma Singleton

import QtQuick
import Bob.Backend

// Colours, sizes and fonts of every view, light or dark as Prefs.theme and the system say.
QtObject {
    readonly property bool dark: Prefs.theme === "dark"
        || (Prefs.theme === "system" && Application.styleHints.colorScheme === Qt.ColorScheme.Dark)

    readonly property color bg: dark ? "#111317" : "#f6f7f9"
    readonly property color surface: dark ? "#181b20" : "#ffffff"
    readonly property color surface2: dark ? "#1f232a" : "#f0f2f5"
    readonly property color surface3: dark ? "#282d35" : "#e7eaef"
    readonly property color border: dark ? "#2c313a" : "#dde1e7"
    readonly property color borderStrong: dark ? "#3b414c" : "#c7cdd6"
    readonly property color text: dark ? "#e7eaee" : "#1b1f24"
    readonly property color text2: dark ? "#b3bac5" : "#4b5563"
    readonly property color text3: dark ? "#858e9c" : "#7a8494"
    readonly property color accent: dark ? "#6a9cf5" : "#2f6fde"
    readonly property color accentSoft: dark ? "#1d2b45" : "#e6eefc"
    readonly property color accentText: dark ? "#a9c5fa" : "#1d4fa8"
    readonly property color ok: dark ? "#4cc584" : "#18864b"
    readonly property color okSoft: dark ? "#173325" : "#e3f4ea"
    readonly property color info: accent
    readonly property color infoSoft: accentSoft
    readonly property color warn: dark ? "#e3a64a" : "#b26b00"
    readonly property color warnSoft: dark ? "#3a2c15" : "#fbf0dc"
    readonly property color error: dark ? "#f0686c" : "#c62a2f"
    readonly property color errorSoft: dark ? "#3d1c1e" : "#fbe5e5"
    readonly property color think: dark ? "#b294f0" : "#7a52c7"
    readonly property color thinkSoft: dark ? "#2a2140" : "#f1ebfb"
    readonly property color shadow: dark ? "#66000000" : "#1f101828"
    readonly property color overlay: "#730a0e14"

    readonly property int radius: 10
    readonly property int radiusSmall: 6
    readonly property int fontSize: 14
    readonly property int smallSize: 12
    readonly property string mono: Desktop.monoFamily
    // The chat column's width: the thread, the requests and the composer
    readonly property int column: 860

    // A colour by tone name, for chips, dots and texts: ok, info, warn, error.
    function tone(name) {
        switch (name) {
        case "ok": return ok
        case "info": return info
        case "warn": return warn
        case "error": return error
        default: return text3
        }
    }
    function toneSoft(name) {
        switch (name) {
        case "ok": return okSoft
        case "info": return infoSoft
        case "warn": return warnSoft
        case "error": return errorSoft
        default: return surface2
        }
    }
}
