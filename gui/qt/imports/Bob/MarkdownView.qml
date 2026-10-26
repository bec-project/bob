import QtQuick
import Bob.Backend

// Model output as Markdown, selectable; links open in the browser.
TextEdit {
    id: view

    property string markdown

    readOnly: true
    selectByMouse: true
    textFormat: TextEdit.RichText
    wrapMode: TextEdit.Wrap
    color: Theme.text
    selectionColor: Theme.accentSoft
    selectedTextColor: Theme.text
    font.pixelSize: 14
    onLinkActivated: (link) => Desktop.openExternal(link)

    MarkdownFormatter {
        document: view.textDocument
        markdown: view.markdown
        linkColor: Theme.accentText
        codeBackground: Theme.surface3
        quoteColor: Theme.text2
    }

    HoverHandler {
        cursorShape: view.hoveredLink ? Qt.PointingHandCursor : Qt.IBeamCursor
    }
}
