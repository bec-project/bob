#pragma once

#include <QObject>
#include <QQmlEngine>
#include <QVariantMap>

// What the window asks of the desktop: links, the clipboard, saving files, and the formatting
// helpers the views share.
class Desktop : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    // The system's monospace font, for code
    Q_PROPERTY(QString monoFamily READ monoFamily CONSTANT)

public:
    using QObject::QObject;

    QString monoFamily() const;

    // Opens an http(s) link in the default browser; returns false for any other link.
    Q_INVOKABLE bool openExternal(const QString &url);
    Q_INVOKABLE void copyText(const QString &text);
    // Saves text as a plain file name (no folders, nothing hidden) in the user's Downloads folder
    // (or home), numbering the name when it is taken. Returns {path} or {error}.
    Q_INVOKABLE QVariantMap saveDownload(const QString &name, const QString &text);
    // A local path from a file dialog's URL.
    Q_INVOKABLE QString localPath(const QUrl &url) const;
    Q_INVOKABLE QString fileName(const QString &path) const;

    Q_INVOKABLE QString formatTokens(double value) const;
    Q_INVOKABLE QString formatDuration(double ms) const;
    Q_INVOKABLE QString formatCost(double cost) const;
    Q_INVOKABLE QString relativeTime(double ms) const;
    // Milliseconds of an ISO 8601 time (as the engines report it), or 0.
    Q_INVOKABLE double parseTime(const QString &iso) const;
    Q_INVOKABLE int clampWidth(double width, int min, int windowWidth) const;
    Q_INVOKABLE QString patchHtml(const QString &patch, const QString &addColor, const QString &deleteColor,
                                  const QString &hunkColor) const;
    // Reads a pasted connection: {url, password, ...}, or an empty map when it is none.
    Q_INVOKABLE QVariantMap parseConnection(const QString &text, const QString &password) const;
};
