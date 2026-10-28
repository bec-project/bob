#include "bob.h"

#include "bobpaths.h"

namespace {
Bob *instance = nullptr;
}

Bob::Bob(const QString &settingsPath, QObject *parent)
    : QObject(parent)
    , m_settings(settingsPath)
{
    connect(&m_session, &BobSession::statusChanged, this, &Bob::statusChanged);
}

Bob *Bob::create(QQmlEngine *, QJSEngine *)
{
    QJSEngine::setObjectOwnership(instance, QJSEngine::CppOwnership);
    return instance;
}

void Bob::setInstance(Bob *bob)
{
    instance = bob;
}

QString Bob::defaultConfig() const
{
    return BobPaths::defaultConfig();
}

QString Bob::defaultBinary() const
{
    return BobPaths::defaultBinary();
}

QString Bob::version() const
{
    return QStringLiteral(BOB_VERSION);
}

void Bob::joinOnLaunch()
{
    const QString config = m_settings.resolvedConfig();
    const QString binary = m_settings.resolvedBinary();
    if (m_settings.get().autoJoin && !config.isEmpty() && !binary.isEmpty())
        m_session.join(binary, config);
}

void Bob::join()
{
    m_session.join(m_settings.resolvedBinary(), m_settings.resolvedConfig());
}

void Bob::leave()
{
    m_session.leave();
}

void Bob::restart()
{
    const QString config = m_settings.resolvedConfig();
    m_session.restart(m_settings.resolvedBinary(), config,
                      QStringLiteral("another app restarted bob with %1").arg(config));
}

QString Bob::saveSettings(const QString &config, const QString &binary, bool autoJoin)
{
    const QString error = m_settings.set({config, binary, autoJoin});
    emit settingsChanged();
    return error;
}
