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

namespace {

const QByteArray up =
    R"({"connection":{"url":"http://127.0.0.1:1","password":"p","name":"fake-1"},"config":"CONFIG","consumers":1})";

// A broker that answers every client with `reply` (CONFIG replaced by `config`), or never answers
// when `reply` is empty, and records the hellos and ended leases it saw.
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
                        hellos << client->readLine();
                        if (!m_reply.isEmpty())
                            client->write(m_reply + '\n');
                    }
                });
                connect(client, &QLocalSocket::disconnected, this, [this] { ++ended; });
            }
        });
    }

    // Closes every lease, as a broker does when bob stops.
    void closeAll()
    {
        for (QLocalSocket *client : std::as_const(m_clients))
            client->disconnectFromServer();
    }

    QList<QByteArray> hellos;
    int ended = 0;

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
        QCOMPARE(joined.name, QStringLiteral("fake-1"));
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
        QCOMPARE(session.status().name, QStringLiteral("fake-1"));

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
        session.join(fakeBob(QStringLiteral("failing"), "#!/bin/sh\necho 'find bwrap on PATH' >&2\nexit 1\n"),
                     m_config);
        QTRY_COMPARE(session.status().state, BobStatus::Idle);
        QVERIFY2(session.status().error.contains(QStringLiteral("could not start")), qPrintable(session.status().error));
        QVERIFY(session.status().error.contains(QStringLiteral("find bwrap on PATH")));

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

    void describesTheStatusLikeTheTauriApp()
    {
        BobStatus joined;
        joined.state = BobStatus::Joined;
        joined.connection = {{"url", "u"}};
        joined.name = QStringLiteral("c");
        joined.config = QStringLiteral("a");
        joined.runningConfig = QStringLiteral("b");
        const QVariantMap expected{{"state", "joined"}, {"connection", QVariantMap{{"url", "u"}}},
                                   {"name", "c"}, {"config", "a"}, {"runningConfig", "b"}};
        QCOMPARE(joined.toVariant(), expected);
        const QVariantMap idle = BobStatus::idle().toVariant();
        QCOMPARE(idle.value("state").toString(), QStringLiteral("idle"));
        QVERIFY(idle.contains("error") && idle.value("error").isNull());
    }
};

QTEST_GUILESS_MAIN(TestBobSession)
#include "tst_bobsession.moc"
