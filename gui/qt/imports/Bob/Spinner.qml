import QtQuick
import QtQuick.Shapes

// A small spinning ring, for work in progress.
Item {
    id: spinner

    property int size: 14
    property real thickness: 2

    implicitWidth: size
    implicitHeight: size

    Shape {
        anchors.fill: parent
        preferredRendererType: Shape.CurveRenderer
        ShapePath {
            strokeColor: Theme.borderStrong
            strokeWidth: spinner.thickness
            fillColor: "transparent"
            PathAngleArc {
                centerX: spinner.size / 2
                centerY: spinner.size / 2
                radiusX: (spinner.size - spinner.thickness) / 2
                radiusY: radiusX
                startAngle: 0
                sweepAngle: 360
            }
        }
        ShapePath {
            strokeColor: Theme.accent
            strokeWidth: spinner.thickness
            fillColor: "transparent"
            capStyle: ShapePath.FlatCap
            PathAngleArc {
                centerX: spinner.size / 2
                centerY: spinner.size / 2
                radiusX: (spinner.size - spinner.thickness) / 2
                radiusY: radiusX
                startAngle: -90
                sweepAngle: 90
            }
        }
        RotationAnimator on rotation {
            from: 0
            to: 360
            duration: 800
            loops: Animation.Infinite
            running: spinner.visible
        }
    }
}
