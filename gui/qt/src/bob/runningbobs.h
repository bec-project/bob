#pragma once

#include <QJsonObject>
#include <QObject>
#include <QVariantList>

#include <optional>

// A bob container found on an engine, with what the app needs to connect to it.
struct RunningBob
{
    QString url;
    QString password;
    QString container;
    QString runtime;
    QString workspace;
    QString image;
    QString started;

    QVariantMap toVariant() const;
};

// The connection details of one `docker inspect` entry, if it is a running bob (a container with
// bob's io.bob.launch label). Its OpenCode password is read back from the container's environment,
// which only someone who can already control the engine can do.
std::optional<RunningBob> runningBob(const QJsonObject &entry, const QString &runtime);

// Lists every bob container on Docker and Podman (`ps`, then `inspect`), in the background.
class RunningBobLister : public QObject
{
    Q_OBJECT
public:
    using QObject::QObject;

    bool listing() const { return m_pending > 0; }
    QList<RunningBob> found() const { return m_found; }
    // Starts listing again; finished() follows once every engine answered.
    void refresh();

signals:
    void finished();

private:
    void inspect(const QString &runtime, const QString &engine, const QStringList &ids, quint64 round);
    void done(quint64 round);

    QList<RunningBob> m_found;
    QList<RunningBob> m_collecting;
    int m_pending = 0;
    quint64 m_round = 0;
};
