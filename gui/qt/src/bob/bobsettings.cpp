#include "bobsettings.h"

#include "bobpaths.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>

namespace {

BobSettings normalized(BobSettings settings)
{
    settings.config = settings.config.trimmed();
    settings.binary = settings.binary.trimmed();
    return settings;
}

}  // namespace

BobSettingsStore::BobSettingsStore(const QString &path)
    : m_path(path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return;
    QJsonParseError error;
    const QJsonDocument document = QJsonDocument::fromJson(file.readAll(), &error);
    if (!document.isObject()) {
        qWarning("ignoring %s: %s", qPrintable(path), qPrintable(error.errorString()));
        return;
    }
    const QJsonObject object = document.object();
    BobSettings settings;
    settings.config = object.value(QStringLiteral("config")).toString();
    settings.binary = object.value(QStringLiteral("binary")).toString();
    settings.autoJoin = object.value(QStringLiteral("autoJoin")).toBool(true);
    m_current = normalized(settings);
}

QString BobSettingsStore::set(BobSettings settings)
{
    settings = normalized(settings);
    const QJsonObject object{
        {QStringLiteral("config"), settings.config.isEmpty() ? QJsonValue() : QJsonValue(settings.config)},
        {QStringLiteral("binary"), settings.binary.isEmpty() ? QJsonValue() : QJsonValue(settings.binary)},
        {QStringLiteral("autoJoin"), settings.autoJoin},
    };
    QDir().mkpath(QFileInfo(m_path).absolutePath());
    // written through a temporary file and a rename, so a crash never leaves half a file
    QSaveFile file(m_path);
    if (!file.open(QIODevice::WriteOnly)
        || file.write(QJsonDocument(object).toJson(QJsonDocument::Indented)) < 0 || !file.commit()) {
        return QStringLiteral("cannot save %1: %2").arg(m_path, file.errorString());
    }
    m_current = settings;
    return {};
}

QString BobSettingsStore::resolvedBinary() const
{
    return m_current.binary.isEmpty() ? BobPaths::defaultBinary() : m_current.binary;
}

QString BobSettingsStore::resolvedConfig() const
{
    return m_current.config.isEmpty() ? BobPaths::defaultConfig() : m_current.config;
}
