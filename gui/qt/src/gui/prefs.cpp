#include "prefs.h"

namespace {
Prefs *instance = nullptr;
}

Prefs::Prefs(QObject *parent)
    : QObject(parent)
{
}

Prefs *Prefs::create(QQmlEngine *, QJSEngine *)
{
    QJSEngine::setObjectOwnership(instance, QJSEngine::CppOwnership);
    return instance;
}

void Prefs::setInstance(Prefs *prefs)
{
    instance = prefs;
}

void Prefs::write(const QString &key, const QVariant &value)
{
    if (m_settings.value(key) == value)
        return;
    if (value.isNull())
        m_settings.remove(key);
    else
        m_settings.setValue(key, value);
    emit changed();
}

QString Prefs::theme() const
{
    return m_settings.value(QStringLiteral("theme"), QStringLiteral("system")).toString();
}

void Prefs::setTheme(const QString &theme)
{
    write(QStringLiteral("theme"), theme);
}

QString Prefs::model() const
{
    return m_settings.value(QStringLiteral("model")).toString();
}

void Prefs::setModel(const QString &model)
{
    write(QStringLiteral("model"), model.isEmpty() ? QVariant() : QVariant(model));
}

bool Prefs::thinking() const
{
    return m_settings.value(QStringLiteral("thinking"), true).toBool();
}

void Prefs::setThinking(bool thinking)
{
    write(QStringLiteral("thinking"), thinking);
}

QString Prefs::agent() const
{
    return m_settings.value(QStringLiteral("agent")).toString();
}

void Prefs::setAgent(const QString &agent)
{
    write(QStringLiteral("agent"), agent.isEmpty() ? QVariant() : QVariant(agent));
}

bool Prefs::sidebar() const
{
    return m_settings.value(QStringLiteral("sidebar"), true).toBool();
}

void Prefs::setSidebar(bool sidebar)
{
    write(QStringLiteral("sidebar"), sidebar);
}

QString Prefs::panel() const
{
    return m_settings.value(QStringLiteral("panel")).toString();
}

void Prefs::setPanel(const QString &panel)
{
    write(QStringLiteral("panel"), panel.isEmpty() ? QVariant() : QVariant(panel));
}

QVariantMap Prefs::widths() const
{
    // an INI file gives the numbers back as text
    QVariantMap widths = m_settings.value(QStringLiteral("widths")).toMap();
    for (auto it = widths.begin(); it != widths.end(); ++it)
        it.value() = it.value().toInt();
    return widths;
}

QVariantMap Prefs::plugins() const
{
    // an INI file gives the switches back as text
    QVariantMap switches = m_settings.value(QStringLiteral("plugins")).toMap();
    for (auto it = switches.begin(); it != switches.end(); ++it)
        it.value() = it.value().toBool();
    return switches;
}

void Prefs::setPlugins(const QVariantMap &plugins)
{
    write(QStringLiteral("plugins"), plugins.isEmpty() ? QVariant() : QVariant(plugins));
}

QVariantMap Prefs::pluginSettings() const
{
    return m_settings.value(QStringLiteral("pluginSettings")).toMap();
}

void Prefs::setPluginSettings(const QVariantMap &settings)
{
    write(QStringLiteral("pluginSettings"), settings.isEmpty() ? QVariant() : QVariant(settings));
}

void Prefs::setWidth(const QString &key, int width)
{
    QVariantMap all = widths();
    if (width < 0)
        all.remove(key);
    else
        all.insert(key, width);
    write(QStringLiteral("widths"), all.isEmpty() ? QVariant() : QVariant(all));
}
