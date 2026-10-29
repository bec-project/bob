#pragma once

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
    // Joined: the bob.toml bob runs with; another one than `config` when another program started it
    QString runningConfig;
    // Lost: why the lease ended
    QString reason;

    static BobStatus idle(const QString &error = {});
    QString stateName() const;
    // {state: "idle", error}, {state: "joining", config}, {state: "joined", connection, name,
    // config, runningConfig} or {state: "lost", reason, config, name}
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
    // status reports the outcome. Holding bob already, or joining, it does nothing.
    void join(const QString &binary, const QString &config);
    // Ends the lease, or the join in progress. bob itself stops when no other client holds it.
    void leave();

    // bob's own startup limit is 90 s, and the broker may first stop the previous bob.
    void setHelloTimeout(int milliseconds) { m_helloTimeout = milliseconds; }

signals:
    void statusChanged();

private:
    void setStatus(const BobStatus &status);
    void fail(quint64 generation, const QString &error);
    void dropProcess();
    void dropLease();
    void located(quint64 generation);
    void connectBroker(quint64 generation, const QString &socket);
    void leaseData(quint64 generation);
    void leaseEnded(quint64 generation, const QString &error);
    void handleReply(quint64 generation, const QByteArray &line);

    BobStatus m_status;
    quint64 m_generation = 0;
    QPointer<QProcess> m_process;
    QPointer<QLocalSocket> m_lease;
    QByteArray m_buffer;
    bool m_greeted = false;
    QTimer m_helloTimer;
    int m_helloTimeout = 180000;
};
