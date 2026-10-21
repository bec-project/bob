#pragma once

#include <QElapsedTimer>
#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QTimer>
#include <QVariantMap>

class QLocalSocket;
class QProcess;

// Where the app stands with the user's bob. The UI shows it and changes it only through
// BobSession::join and BobSession::leave.
struct BobStatus
{
    enum State { Idle, Joining, Joined, Lost };
    State state = Idle;
    // Idle: why the last join failed (empty when it did not)
    QString error;
    // Joining, Joined, Lost: the bob.toml this app joins with
    QString config;
    // Joined: bob's connection (url, password, name, ...), as the broker sent it
    QJsonObject connection;
    // Joined, Lost
    QString name;
    // Joined: the bob.toml bob runs with; another one than `config` (the one this app asked for)
    // when another program started it first
    QString runningConfig;
    // Joined: the clients holding bob when this app joined, this one included
    int consumers = 0;
    // Lost: why the lease ended
    QString reason;

    static BobStatus idle(const QString &error = {});
    QString stateName() const;
    // {state: "idle", error}, {state: "joining", config}, {state: "joined", connection, name,
    // config, runningConfig, consumers} or {state: "lost", reason, config, name}
    QVariantMap toVariant() const;
    bool operator==(const BobStatus &other) const;
    bool operator!=(const BobStatus &other) const { return !(*this == other); }
};

// The app's one lease on the user's bob.
//
// bob runs once per user, owned by a broker process. `bob-manager --config bob.toml` makes sure
// the broker runs, starting it when none does, and prints its socket. The app connects there,
// says hello and gets bob's connection once bob is up. It holds its lease for as long as it keeps
// that connection open, so leaving bob, quitting and crashing all end it; the broker stops bob
// 90 s after the last client is gone. When the broker closes the connection (bob stopped or
// failed), the session is lost.
//
// Everything runs on the thread that owns the session; every join and leave bumps a generation,
// so work that was overtaken never changes newer state.
class BobSession : public QObject
{
    Q_OBJECT
public:
    explicit BobSession(QObject *parent = nullptr);
    ~BobSession() override;

    BobStatus status() const { return m_status; }

    // Joins the user's bob, starting it with this bob.toml when none runs. Returns at once; the
    // status reports the outcome. Joining, it does nothing; holding bob already, it keeps it but
    // notes the bob.toml asked for, so a different one shows as a mismatch.
    void join(const QString &binary, const QString &config);
    // Stops the running bob for every client, then starts it again with this bob.toml and joins
    // it. The other clients' leases end with `reason`.
    void restart(const QString &binary, const QString &config, const QString &reason);
    // Ends the lease, or the join in progress. bob itself stops when no other client holds it.
    void leave();

    // bob's own startup limit is 90 s, and the broker may first stop the previous bob.
    void setHelloTimeout(int milliseconds) { m_helloTimeout = milliseconds; }
    // How long a stopped broker may take to go away.
    void setStopTimeout(int milliseconds) { m_stopTimeout = milliseconds; }

signals:
    void statusChanged();

private:
    void setStatus(const BobStatus &status);
    void fail(quint64 generation, const QString &error);
    void dropProcess();
    void dropLease();
    void dropStop();
    void start(const QString &binary, const QString &config, const QString &stopReason);
    void locate(quint64 generation);
    void located(quint64 generation);
    void stopBroker(quint64 generation, const QString &socket);
    void connectBroker(quint64 generation, const QString &socket);
    void leaseData(quint64 generation);
    void leaseEnded(quint64 generation, const QString &error);
    void handleReply(quint64 generation, const QByteArray &line);

    BobStatus m_status;
    quint64 m_generation = 0;
    QString m_binary;
    // While restarting: why the running bob stops, until its broker went away
    QString m_stopReason;
    // What the broker said when it ended the lease
    QString m_stoppedReason;
    QPointer<QLocalSocket> m_stopRequest;
    QTimer m_stopWatch;
    QElapsedTimer m_stopWaited;
    int m_stopTimeout = 60000;
    QPointer<QProcess> m_process;
    QPointer<QLocalSocket> m_lease;
    QByteArray m_buffer;
    bool m_greeted = false;
    QTimer m_helloTimer;
    int m_helloTimeout = 180000;
};
