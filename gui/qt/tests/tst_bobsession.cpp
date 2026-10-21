// The lease on bob, against fake bob-manager scripts and a fake broker on a Unix socket.

#include "bobsession.h"

#include <QElapsedTimer>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QLocalServer>
#include <QLocalSocket>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTest>

#include <functional>
#include <memory>

namespace {

const QByteArray up =
    R"({"connection":{"url":"http://127.0.0.1:1","password":"p","container":"fake-1"},"config":"CONFIG","consumers":1})";

// A broker that answers every client with `reply` (CONFIG replaced by `config`), or never answers
// when `reply` is empty, and records the hellos and ended leases it saw. A stop request makes it
// tell every lease why, close them and go away, as the real one does.
class FakeBroker : public QObject
{
public:
    FakeBroker(const QString &path, const QByteArray &reply, const QString &config)
        : m_reply(QByteArray(reply).replace("CONFIG", config.toUtf8()))
    {
        QLocalServer::removeServer(path);
        QVERIFY(m_server.listen(path));
        connect(&m_server, &QLocalServer::newConnection, this, [this] {
            while (QLocalSocket *client = m_server.nextPendingConnection()) {
                m_clients << client;
                connect(client, &QLocalSocket::readyRead, this, [this, client] {
                    while (client->canReadLine()) {
                        const QByteArray line = client->readLine();
                        const QJsonObject hello = QJsonDocument::fromJson(line).object();
                        if (hello.value("stop").toBool()) {
                            if (ignoreStop)
                                continue;
                            const QString reason = hello.value("reason").toString();
                            stops << reason;
                            m_clients.removeAll(client);
                            client->write(QJsonDocument(QJsonObject{{"stopped", reason}}).toJson(QJsonDocument::Compact) + '\n');
                            closeAll(reason);
                            m_server.close();
                            if (afterStop)
                                afterStop();
                            return;
                        }
                        hellos << line;
                        if (!m_reply.isEmpty())
                            client->write(m_reply + '\n');
                    }
                });
                connect(client, &QLocalSocket::disconnected, this, [this] { ++ended; });
            }
        });
    }

    // Closes every lease, as a broker does when bob stops, telling each why first when there is
    // a reason.
    void closeAll(const QString &reason = {})
    {
        for (QLocalSocket *client : std::as_const(m_clients)) {
            if (!reason.isEmpty())
                client->write(QJsonDocument(QJsonObject{{"stopped", reason}}).toJson(QJsonDocument::Compact) + '\n');
            client->disconnectFromServer();
        }
    }

    QList<QByteArray> hellos;
    QStringList stops;
    int ended = 0;
    bool ignoreStop = false;
    std::function<void()> afterStop;

private:
    QLocalServer m_server;
    QByteArray m_reply;
    QList<QLocalSocket *> m_clients;
};

}  // namespace

class TestBobSession : public QObject
{
    Q_OBJECT

private:
    QTemporaryDir m_dir{QStringLiteral("/tmp/bobqt-XXXXXX")};
    QString m_config;

    // A stand-in for bob-manager.
    QString fakeBob(const QString &name, const QByteArray &script)
    {
        const QString path = m_dir.filePath(name);
        QFile file(path);
        if (!file.open(QIODevice::WriteOnly))
            return {};
        file.write(script);
        file.close();
        file.setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner | QFileDevice::ExeOwner);
        return path;
    }

    // A stand-in for bob-manager that prints the socket of a broker the test runs.
    QString printingSocket(const QString &socket)
    {
        return fakeBob(QStringLiteral("bob-%1").arg(QFileInfo(socket).baseName()),
                       QStringLiteral("#!/bin/sh\necho '{\"socket\":\"%1\"}'\n").arg(socket).toUtf8());
    }

private slots:
    void initTestCase()
    {
        QVERIFY(m_dir.isValid());
        m_config = m_dir.filePath(QStringLiteral("bob.toml"));
        QFile config(m_config);
        QVERIFY(config.open(QIODevice::WriteOnly));
    }

    void holdsOneLeaseUntilLeft()
    {
        const QString socket = m_dir.filePath(QStringLiteral("lease.sock"));
        FakeBroker broker(socket, up, m_config);
        BobSession session;
        QSignalSpy changes(&session, &BobSession::statusChanged);
        const QString binary = printingSocket(socket);

        session.join(binary, m_config);
        QCOMPARE(session.status().state, BobStatus::Joining);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        const BobStatus joined = session.status();
        QCOMPARE(joined.connection.value("url").toString(), QStringLiteral("http://127.0.0.1:1"));
        QCOMPARE(joined.container, QStringLiteral("fake-1"));
        QCOMPARE(joined.runningConfig, m_config);
        QCOMPARE(broker.hellos.size(), 1);
        const QJsonObject hello = QJsonDocument::fromJson(broker.hellos.first()).object();
        QCOMPARE(hello.value("consumer").toString(), QStringLiteral("bob-gui"));
        QCOMPARE(hello.value("config").toString(), m_config);
        QCOMPARE(hello.value("pid").toInteger(), QCoreApplication::applicationPid());

        // joining again keeps the one lease
        session.join(binary, m_config);
        QTest::qWait(200);
        QCOMPARE(broker.hellos.size(), 1);
        QCOMPARE(session.status(), joined);

        const int before = int(changes.size());
        session.leave();
        QCOMPARE(session.status(), BobStatus::idle());
        QTRY_COMPARE(broker.ended, 1);
        // leaving ends the lease quietly: no lost status follows
        QTest::qWait(200);
        QCOMPARE(changes.size(), before + 1);
        QCOMPARE(session.status(), BobStatus::idle());
    }

    void reportsALostBobAndJoinsAgain()
    {
        const QString socket = m_dir.filePath(QStringLiteral("lost.sock"));
        FakeBroker broker(socket, up, m_config);
        BobSession session;
        const QString binary = printingSocket(socket);
        session.join(binary, m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);

        broker.closeAll();
        QTRY_COMPARE(session.status().state, BobStatus::Lost);
        QCOMPARE(session.status().reason, QStringLiteral("bob stopped: its broker ended this app's lease"));
        QCOMPARE(session.status().config, m_config);
        QCOMPARE(session.status().container, QStringLiteral("fake-1"));

        session.join(binary, m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QCOMPARE(broker.hellos.size(), 2);
        session.leave();
    }

    void reportsBobRunningWithAnotherConfig()
    {
        const QString socket = m_dir.filePath(QStringLiteral("other.sock"));
        FakeBroker broker(socket, up, QStringLiteral("/elsewhere/bob.toml"));
        BobSession session;
        session.join(printingSocket(socket), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QCOMPARE(session.status().config, m_config);
        QCOMPARE(session.status().runningConfig, QStringLiteral("/elsewhere/bob.toml"));
        session.leave();
    }

    void leavingEndsAJoinInProgress()
    {
        const QString socket = m_dir.filePath(QStringLiteral("cancel.sock"));
        // a broker that never answers: bob is still starting
        FakeBroker broker(socket, QByteArray(), m_config);
        BobSession session;
        session.join(printingSocket(socket), m_config);
        QTRY_COMPARE(broker.hellos.size(), 1);
        QCOMPARE(session.status().state, BobStatus::Joining);
        session.leave();
        QCOMPARE(session.status(), BobStatus::idle());
        QTRY_COMPARE(broker.ended, 1);
    }

    void leavingStopsBobManager()
    {
        // bob-manager still waiting for a broker to answer
        BobSession session;
        session.join(fakeBob(QStringLiteral("sleepy"), "#!/bin/sh\nexec sleep 30\n"), m_config);
        QCOMPARE(session.status().state, BobStatus::Joining);
        QElapsedTimer timer;
        timer.start();
        session.leave();
        QVERIFY(timer.elapsed() < 5000);
        QCOMPARE(session.status(), BobStatus::idle());
    }

    void aSecondJoinFindsTheFirst()
    {
        const QString socket = m_dir.filePath(QStringLiteral("wait.sock"));
        FakeBroker broker(socket, up, m_config);
        // bob-manager takes a moment, so the second join finds the first one running
        const QString binary = fakeBob(QStringLiteral("slow"),
                                       QStringLiteral("#!/bin/sh\nsleep 0.5\necho '{\"socket\":\"%1\"}'\n").arg(socket).toUtf8());
        BobSession session;
        session.join(binary, m_config);
        session.join(binary, m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QTest::qWait(200);
        QCOMPARE(broker.hellos.size(), 1);
        session.leave();
    }

    void reportsAStartFailure()
    {
        BobSession session;
        session.join(fakeBob(QStringLiteral("failing"), "#!/bin/sh\necho 'start the docker engine first' >&2\nexit 1\n"),
                     m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QVERIFY2(session.status().error.contains(QStringLiteral("could not start")), qPrintable(session.status().error));
        QVERIFY(session.status().error.contains(QStringLiteral("start the docker engine first")));

        session.join(fakeBob(QStringLiteral("unused"), "#!/bin/sh\n"), QStringLiteral("/nonexistent/bob.toml"));
        QCOMPARE(session.status().state, BobStatus::Idle);
        QCOMPARE(session.status().error, QStringLiteral("/nonexistent/bob.toml is not a file"));

        session.join(m_dir.filePath(QStringLiteral("bob.toml")), m_config);
        QVERIFY(session.status().error.endsWith(QStringLiteral("is not an executable")));

        session.join(QString(), m_config);
        QVERIFY(session.status().error.startsWith(QStringLiteral("No bob executable")));

        session.join(QStringLiteral("/usr/bin/true"), QString());
        QCOMPARE(session.status().error, QStringLiteral("Choose bob's configuration file (bob.toml) first."));

        // bob that failed in the broker: the client gets the broker's error
        const QString socket = m_dir.filePath(QStringLiteral("error.sock"));
        FakeBroker broker(socket, R"({"error":"no space left on device"})", QString());
        session.join(printingSocket(socket), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QCOMPARE(session.status().error, QStringLiteral("no space left on device"));
        QTRY_COMPARE(broker.ended, 1);
    }

    void reportsABrokerThatIsNotThere()
    {
        BobSession session;
        session.join(printingSocket(m_dir.filePath(QStringLiteral("absent.sock"))), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QVERIFY2(session.status().error.startsWith(QStringLiteral("cannot reach bob's broker at")),
                 qPrintable(session.status().error));

        session.join(fakeBob(QStringLiteral("silent"), "#!/bin/sh\necho nothing\n"), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QVERIFY(session.status().error.startsWith(QStringLiteral("bob printed no broker socket")));
    }

    void givesUpOnABrokerThatNeverAnswers()
    {
        const QString socket = m_dir.filePath(QStringLiteral("mute.sock"));
        FakeBroker broker(socket, QByteArray(), m_config);
        BobSession session;
        session.setHelloTimeout(300);
        session.join(printingSocket(socket), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QVERIFY(session.status().error.startsWith(QStringLiteral("bob's broker did not answer")));
        QTRY_COMPARE(broker.ended, 1);
    }

    void reportsWhyTheBrokerEndedTheLease()
    {
        const QString socket = m_dir.filePath(QStringLiteral("reason.sock"));
        FakeBroker broker(socket, up, m_config);
        BobSession session;
        session.join(printingSocket(socket), m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QCOMPARE(session.status().consumers, 1);
        broker.closeAll(QStringLiteral("bob stopped: its container is gone"));
        QTRY_COMPARE(session.status().state, BobStatus::Lost);
        QCOMPARE(session.status().reason, QStringLiteral("bob stopped: its container is gone"));
    }

    void joiningAgainWithAnotherConfigShowsTheMismatch()
    {
        const QString socket = m_dir.filePath(QStringLiteral("mismatch.sock"));
        const QString other = m_dir.filePath(QStringLiteral("other.toml"));
        QVERIFY(QFile(other).open(QIODevice::WriteOnly));
        FakeBroker broker(socket, up, m_config);
        BobSession session;
        const QString binary = printingSocket(socket);
        session.join(binary, m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QSignalSpy changes(&session, &BobSession::statusChanged);
        session.join(binary, other);
        // the window hears about it, and the lease stays the one
        QCOMPARE(changes.size(), 1);
        QCOMPARE(session.status().config, other);
        QCOMPARE(session.status().runningConfig, m_config);
        QTest::qWait(200);
        QCOMPARE(broker.hellos.size(), 1);
        session.leave();
    }

    void restartingStopsTheBrokerAndJoinsANewBob()
    {
        const QString socket = m_dir.filePath(QStringLiteral("restart.sock"));
        const QString other = m_dir.filePath(QStringLiteral("other.toml"));
        QVERIFY(QFile(other).open(QIODevice::WriteOnly));
        const QString binary = printingSocket(socket);
        // the first broker runs bob with bob.toml; once it has gone, a second one runs other.toml
        FakeBroker first(socket, up, m_config);
        std::unique_ptr<FakeBroker> second;
        first.afterStop = [&] { second = std::make_unique<FakeBroker>(socket, up, other); };
        BobSession session;
        session.join(binary, m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);

        session.restart(binary, other, QStringLiteral("another app restarted bob with other.toml"));
        QCOMPARE(session.status().state, BobStatus::Joining);
        QTRY_COMPARE(session.status().state, BobStatus::Joined);
        QCOMPARE(first.stops, QStringList{"another app restarted bob with other.toml"});
        QVERIFY(second);
        QCOMPARE(second->hellos.size(), 1);
        QVERIFY(second->hellos.first().contains("other.toml"));
        QCOMPARE(session.status().config, other);
        QCOMPARE(session.status().runningConfig, other);
        session.leave();
    }


    void givesUpOnABrokerThatDoesNotStop()
    {
        const QString socket = m_dir.filePath(QStringLiteral("stubborn.sock"));
        FakeBroker broker(socket, up, m_config);
        broker.ignoreStop = true;
        BobSession session;
        session.setStopTimeout(300);
        session.restart(printingSocket(socket), m_config, QStringLiteral("why"));
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QCOMPARE(session.status().error, QStringLiteral("bob's broker did not stop"));
    }

    void describesTheStatusLikeTheTauriApp()
    {
        BobStatus joined;
        joined.state = BobStatus::Joined;
        joined.connection = {{"url", "u"}};
        joined.container = QStringLiteral("c");
        joined.config = QStringLiteral("a");
        joined.runningConfig = QStringLiteral("b");
        joined.consumers = 2;
        const QVariantMap expected{{"state", "joined"}, {"connection", QVariantMap{{"url", "u"}}}, {"container", "c"},
                                   {"config", "a"}, {"runningConfig", "b"}, {"consumers", 2}};
        QCOMPARE(joined.toVariant(), expected);
        const QVariantMap idle = BobStatus::idle().toVariant();
        QCOMPARE(idle.value("state").toString(), QStringLiteral("idle"));
        QVERIFY(idle.contains("error") && idle.value("error").isNull());
    }
};

QTEST_GUILESS_MAIN(TestBobSession)
#include "tst_bobsession.moc"
