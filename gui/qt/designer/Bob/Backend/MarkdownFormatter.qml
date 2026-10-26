import QtQuick

// Design Studio's stand-in for the app's Markdown renderer (src/gui/markdowntext.h): it hands the text
// to the TextEdit it sits in, as Qt's own Markdown.
Item {
    property var document
    property string markdown
    property color linkColor
    property color codeBackground
    property color quoteColor

    function show() {
        if (!parent)
            return
        parent.textFormat = TextEdit.MarkdownText
        parent.text = markdown
    }
    onMarkdownChanged: show()
    Component.onCompleted: show()
}
