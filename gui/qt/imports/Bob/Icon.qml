import QtQuick
import QtQuick.Shapes

// A line icon on a 24×24 grid, drawn in `color` (the web UI's icon set).
Item {
    id: icon

    property string name
    property int size: 16
    property color color: Theme.text2

    readonly property var paths: ({
        "plus": "M12 5v14M5 12h14",
        "send": "M5 12h13M13 6l6 6-6 6",
        "stop": "M8 6h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-8a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2z",
        "trash": "M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3",
        "edit": "M4 20h4L19 9l-4-4L4 16v4zM14 6l4 4",
        "settings": "M15 12a3 3 0 1 1-6 0a3 3 0 1 1 6 0z"
            + "M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1 .1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8 .3l-.1 .1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1 .1a1.7 1.7 0 0 0 1.8 .3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1 .1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z",
        "chevron": "M9 6l6 6-6 6",
        "check": "M5 12l5 5L20 7",
        "x": "M6 6l12 12M18 6L6 18",
        "brain": "M9 4a3 3 0 0 0-3 3v.5A3 3 0 0 0 4 10.4 3 3 0 0 0 5 15a3 3 0 0 0 4 4.8V4z"
            + "M15 4a3 3 0 0 1 3 3v.5a3 3 0 0 1 2 2.9 3 3 0 0 1-1 4.6 3 3 0 0 1-4 4.8V4z",
        "tool": "M14.7 6.3a4 4 0 0 0-5.4 5.4L4 17l3 3 5.3-5.3a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.4-.6-.6-2.4 2.5-2.5z",
        "copy": "M11 9h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2zM5 15V5a2 2 0 0 1 2-2h8",
        "sidebar": "M5 4h14a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zM9 4v16",
        "refresh": "M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7",
        "down": "M12 5v14M6 13l6 6 6-6",
        "clock": "M21 12a9 9 0 1 1-18 0a9 9 0 1 1 18 0zM12 7v5l3 2",
        "shield": "M12 3l8 3v6c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V6l8-3z",
        "question": "M21 12a9 9 0 1 1-18 0a9 9 0 1 1 18 0zM9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6 .3-1 .9-1 1.6V14M12 17.5v.01",
        "alert": "M12 3l9.5 16.5H2.5L12 3zM12 10v4M12 17v.01",
        "file": "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5zM14 3v5h5",
        "undo": "M9 14L4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3",
        "fork": "M8 5a2 2 0 1 1-4 0a2 2 0 1 1 4 0zM20 5a2 2 0 1 1-4 0a2 2 0 1 1 4 0zM14 19a2 2 0 1 1-4 0a2 2 0 1 1 4 0z"
            + "M6 7v2a4 4 0 0 0 4 4h4a4 4 0 0 0 4-4V7M12 13v4",
        "download": "M12 4v11M7 10l5 5 5-5M5 20h14",
        "list": "M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01",
        "compress": "M4 9h5V4M20 9h-5V4M4 15h5v5M20 15h-5v5"
    })

    implicitWidth: size
    implicitHeight: size

    Shape {
        anchors.fill: parent
        preferredRendererType: Shape.CurveRenderer
        ShapePath {
            scale: Qt.size(icon.size / 24, icon.size / 24)
            strokeColor: icon.color
            strokeWidth: 1.8 * icon.size / 24
            fillColor: "transparent"
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            PathSvg { path: icon.paths[icon.name] ?? "" }
        }
    }
}
