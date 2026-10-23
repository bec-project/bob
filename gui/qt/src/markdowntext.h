#pragma once

#include <QColor>
#include <QObject>
#include <QPointer>
#include <QQmlEngine>
#include <QQuickTextDocument>

// Fills a TextEdit's document with model output as Markdown (GitHub's dialect), and colours its
// links and inline code. Raw HTML in the text is shown as text, never rendered.
//
//     TextEdit { id: edit; readOnly: true; textFormat: TextEdit.RichText
//         MarkdownFormatter { document: edit.textDocument; markdown: text } }
class MarkdownFormatter : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QQuickTextDocument *document READ document WRITE setDocument NOTIFY documentChanged)
    Q_PROPERTY(QString markdown READ markdown WRITE setMarkdown NOTIFY markdownChanged)
    Q_PROPERTY(QColor linkColor READ linkColor WRITE setLinkColor NOTIFY styleChanged)
    Q_PROPERTY(QColor codeBackground READ codeBackground WRITE setCodeBackground NOTIFY styleChanged)
    Q_PROPERTY(QColor quoteColor READ quoteColor WRITE setQuoteColor NOTIFY styleChanged)

public:
    using QObject::QObject;

    QQuickTextDocument *document() const { return m_document; }
    void setDocument(QQuickTextDocument *document);
    QString markdown() const { return m_markdown; }
    void setMarkdown(const QString &markdown);
    QColor linkColor() const { return m_linkColor; }
    void setLinkColor(const QColor &color);
    QColor codeBackground() const { return m_codeBackground; }
    void setCodeBackground(const QColor &color);
    QColor quoteColor() const { return m_quoteColor; }
    void setQuoteColor(const QColor &color);

signals:
    void documentChanged();
    void markdownChanged();
    void styleChanged();

private:
    void render();

    QPointer<QQuickTextDocument> m_document;
    QString m_markdown;
    QColor m_linkColor;
    QColor m_codeBackground;
    QColor m_quoteColor;
};
