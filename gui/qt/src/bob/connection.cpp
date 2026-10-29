#include "connection.h"

#include <QJsonDocument>
#include <QUrl>

namespace Connection {

std::optional<QJsonObject> fromJson(const QJsonValue &value)
{
    if (!value.isObject())
        return std::nullopt;
    const QJsonObject object = value.toObject();
    if (!object.value(QStringLiteral("url")).isString() || !object.value(QStringLiteral("password")).isString())
        return std::nullopt;
    return object;
}

std::optional<QJsonObject> parse(const QString &text, const QString &password)
{
    const QString trimmed = text.trimmed();
    if (trimmed.isEmpty())
        return std::nullopt;
    if (trimmed.startsWith(QLatin1Char('{'))) {
        const QJsonDocument document = QJsonDocument::fromJson(trimmed.toUtf8());
        return document.isObject() ? fromJson(document.object()) : std::nullopt;
    }
    const QUrl url(trimmed, QUrl::StrictMode);
    if (!url.isValid() || (url.scheme() != QStringLiteral("http") && url.scheme() != QStringLiteral("https"))
        || url.host().isEmpty()) {
        return std::nullopt;
    }
    QString path = url.path();
    while (path.endsWith(QLatin1Char('/')))
        path.chop(1);
    const QString origin = url.toString(QUrl::RemovePath | QUrl::RemoveQuery | QUrl::RemoveFragment
                                        | QUrl::RemoveUserInfo | QUrl::StripTrailingSlash);
    return QJsonObject{{QStringLiteral("url"), origin + path}, {QStringLiteral("password"), password}};
}

}  // namespace Connection
