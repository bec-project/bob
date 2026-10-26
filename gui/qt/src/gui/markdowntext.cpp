#include "markdowntext.h"

#include <QFontDatabase>
#include <QTextBlock>
#include <QTextCursor>
#include <QTextDocument>

void MarkdownFormatter::setDocument(QQuickTextDocument *document)
{
    if (document == m_document)
        return;
    m_document = document;
    emit documentChanged();
    render();
}

void MarkdownFormatter::setMarkdown(const QString &markdown)
{
    if (markdown == m_markdown)
        return;
    m_markdown = markdown;
    emit markdownChanged();
    render();
}

void MarkdownFormatter::setLinkColor(const QColor &color)
{
    if (color == m_linkColor)
        return;
    m_linkColor = color;
    emit styleChanged();
    render();
}

void MarkdownFormatter::setCodeBackground(const QColor &color)
{
    if (color == m_codeBackground)
        return;
    m_codeBackground = color;
    emit styleChanged();
    render();
}

void MarkdownFormatter::setQuoteColor(const QColor &color)
{
    if (color == m_quoteColor)
        return;
    m_quoteColor = color;
    emit styleChanged();
    render();
}

void MarkdownFormatter::render()
{
    if (!m_document || !m_document->textDocument())
        return;
    QTextDocument *document = m_document->textDocument();
    document->setMarkdown(m_markdown, QTextDocument::MarkdownFeatures(QTextDocument::MarkdownDialectGitHub
                                                                      | QTextDocument::MarkdownNoHTML));
    const QFont mono = QFontDatabase::systemFont(QFontDatabase::FixedFont);
    QTextCursor cursor(document);
    cursor.beginEditBlock();
    for (QTextBlock block = document->begin(); block.isValid(); block = block.next()) {
        // headings stand apart from what precedes them; other gaps stay narrow
        QTextBlockFormat blockFormat = block.blockFormat();
        const bool quote = blockFormat.hasProperty(QTextFormat::BlockQuoteLevel);
        const bool first = block == document->begin();
        if (blockFormat.headingLevel() > 0) {
            blockFormat.setTopMargin(first ? 0 : 12);
            blockFormat.setBottomMargin(4);
        } else {
            blockFormat.setTopMargin(first ? 0 : qMin(blockFormat.topMargin(), 8.0));
            blockFormat.setBottomMargin(qMin(blockFormat.bottomMargin(), 8.0));
        }
        QTextCursor blockCursor(block);
        blockCursor.setBlockFormat(blockFormat);
        for (auto it = block.begin(); !it.atEnd(); ++it) {
            const QTextFragment fragment = it.fragment();
            if (!fragment.isValid())
                continue;
            QTextCharFormat format = fragment.charFormat();
            bool changed = false;
            if (format.isAnchor() && m_linkColor.isValid()) {
                format.setForeground(m_linkColor);
                format.setFontUnderline(true);
                changed = true;
            }
            if (format.fontFixedPitch()) {
                format.setFontFamilies(mono.families());
                if (m_codeBackground.isValid())
                    format.setBackground(m_codeBackground);
                changed = true;
            }
            if (quote && m_quoteColor.isValid()) {
                format.setForeground(m_quoteColor);
                changed = true;
            }
            if (changed) {
                QTextCursor range(document);
                range.setPosition(fragment.position());
                range.setPosition(fragment.position() + fragment.length(), QTextCursor::KeepAnchor);
                range.setCharFormat(format);
            }
        }
    }
    cursor.endEditBlock();
}
