#pragma once

#include "bobsession.h"
#include "bobsettings.h"
#include "runningbobs.h"

#include <QObject>
#include <QQmlEngine>

// The app's side of bob for QML: the lease (BobSession), how bob is started (settings.json) and
// the bob containers running on this machine. One instance, created before the window.
class Bob : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    // {state, ...} as BobStatus::toVariant() describes it
    Q_PROPERTY(QVariantMap status READ status NOTIFY statusChanged)
    Q_PROPERTY(QString state READ state NOTIFY statusChanged)
    Q_PROPERTY(QString config READ config NOTIFY settingsChanged)
    Q_PROPERTY(QString binary READ binary NOTIFY settingsChanged)
    Q_PROPERTY(bool autoJoin READ autoJoin NOTIFY settingsChanged)
    // What an empty field stands for
    Q_PROPERTY(QString defaultConfig READ defaultConfig NOTIFY settingsChanged)
    Q_PROPERTY(QString defaultBinary READ defaultBinary NOTIFY settingsChanged)
    Q_PROPERTY(QVariantList running READ running NOTIFY runningChanged)
    Q_PROPERTY(bool listing READ listing NOTIFY runningChanged)
    Q_PROPERTY(QString version READ version CONSTANT)

public:
    explicit Bob(const QString &settingsPath, QObject *parent = nullptr);

    static Bob *create(QQmlEngine *, QJSEngine *);
    static void setInstance(Bob *bob);

    BobSession *session() { return &m_session; }
    BobStatus bobStatus() const { return m_session.status(); }

    QVariantMap status() const { return m_session.status().toVariant(); }
    QString state() const { return m_session.status().stateName(); }
    QString config() const { return m_settings.get().config; }
    QString binary() const { return m_settings.get().binary; }
    bool autoJoin() const { return m_settings.get().autoJoin; }
    QString defaultConfig() const;
    QString defaultBinary() const;
    QVariantList running() const;
    bool listing() const { return m_lister.listing(); }
    QString version() const;

    // Joins (or starts) bob when the app opens, unless that is switched off.
    void joinOnLaunch();

    // Joins the user's bob with the chosen (or default) executable and bob.toml.
    Q_INVOKABLE void join();
    Q_INVOKABLE void leave();
    // Saves how the app starts bob; it applies from the next join. Returns an error or "".
    Q_INVOKABLE QString saveSettings(const QString &config, const QString &binary, bool autoJoin);
    Q_INVOKABLE void refreshRunning();

signals:
    void statusChanged();
    void settingsChanged();
    void runningChanged();

private:
    BobSession m_session;
    BobSettingsStore m_settings;
    RunningBobLister m_lister;
};
