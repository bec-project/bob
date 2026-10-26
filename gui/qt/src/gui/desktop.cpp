#include "desktop.h"

#include "chatlogic.h"
#include "connection.h"
#include "turns.h"

#include <QClipboard>
#include <QDateTime>
#include <QDesktopServices>
#include <QDir>
#include <QFileInfo>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QRegularExpression>
#include <QSaveFile>
#include <QStandardPaths>
#include <QUrl>

QString Desktop::monoFamily() const
{
    return QFontDatabase::systemFont(QFontDatabase::FixedFont).family();
}

bool Desktop::openExternal(const QString &url)
{
    const QUrl parsed(url);
    if (parsed.scheme() != QStringLiteral("http") && parsed.scheme() != QStringLiteral("https"))
        return false;
    return QDesktopServices::openUrl(parsed);
}

void Desktop::copyText(const QString &text)
{
    QGuiApplication::clipboard()->setText(text);
}

QVariantMap Desktop::saveDownload(const QString &name, const QString &text)
{
    QString plain = name;
    plain.replace(QLatin1Char('/'), QLatin1Char('_')).replace(QLatin1Char('\\'), QLatin1Char('_'));
    while (plain.startsWith(QLatin1Char('.')))
        plain.remove(0, 1);
    if (plain.isEmpty())
        return {{QStringLiteral("error"), QStringLiteral("empty file name")}};
    QString dir = QStandardPaths::writableLocation(QStandardPaths::DownloadLocation);
    if (dir.isEmpty() || !QFileInfo(dir).isDir())
        dir = QDir::homePath();
    const QString path = ChatLogic::freePath(dir, plain);
    QSaveFile file(path);
    if (!file.open(QIODevice::WriteOnly) || file.write(text.toUtf8()) < 0 || !file.commit())
        return {{QStringLiteral("error"), QStringLiteral("cannot write %1: %2").arg(path, file.errorString())}};
    return {{QStringLiteral("path"), path}};
}

QString Desktop::localPath(const QUrl &url) const
{
    return url.isLocalFile() ? url.toLocalFile() : url.toString();
}

QString Desktop::fileName(const QString &path) const
{
    return QFileInfo(path).fileName();
}

QString Desktop::formatTokens(double value) const
{
    return ::formatTokens(value);
}

QString Desktop::formatDuration(double ms) const
{
    return ::formatDuration(ms);
}

QString Desktop::formatCost(double cost) const
{
    return ::formatCost(cost);
}

QString Desktop::relativeTime(double ms) const
{
    return ChatLogic::relativeTime(ms, double(QDateTime::currentMSecsSinceEpoch()));
}

double Desktop::parseTime(const QString &iso) const
{
    // engines report nanoseconds; Qt reads milliseconds
    static const QRegularExpression fraction(QStringLiteral("(\\.\\d{3})\\d+"));
    const QDateTime time = QDateTime::fromString(QString(iso).replace(fraction, QStringLiteral("\\1")), Qt::ISODateWithMs);
    return time.isValid() ? double(time.toMSecsSinceEpoch()) : 0;
}

int Desktop::clampWidth(double width, int min, int windowWidth) const
{
    return ChatLogic::clampWidth(width, min, windowWidth);
}

QString Desktop::patchHtml(const QString &patch, const QString &addColor, const QString &deleteColor,
                           const QString &hunkColor) const
{
    return ChatLogic::patchHtml(patch, addColor, deleteColor, hunkColor);
}

QVariantMap Desktop::parseConnection(const QString &text, const QString &password) const
{
    const auto connection = Connection::parse(text, password);
    return connection ? connection->toVariantMap() : QVariantMap();
}
