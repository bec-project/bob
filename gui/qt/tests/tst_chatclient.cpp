// The chat's side of OpenCode, without QML, against a fake OpenCode server on the loopback interface.

#include "chatclient.h"

#include <QElapsedTimer>
#include <QJsonArray>
#include <QJsonDocument>
#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTest>
#include <QTimer>

namespace {

// Speaks just enough of OpenCode's HTTP API: what ChatClient loads when it opens, the event stream
// (kept open, events pushed by the test), sessions, messages and prompts. Requests are recorded.
class FakeOpencode : public QObject
{
public:
    FakeOpencode()
    {
        QVERIFY(m_server.listen(QHostAddress::LocalHost));
        connect(&m_server, &QTcpServer::newConnection, this, [this] {
            while (QTcpSocket *client = m_server.nextPendingConnection())
                connect(client, &QTcpSocket::readyRead, this, [this, client] { read(client); });
        });
    }

    QJsonObject connection(const QString &password = QStringLiteral("pw")) const
    {
        return {{"url", QStringLiteral("http://127.0.0.1:%1").arg(m_server.serverPort())}, {"password", password}};
    }

    // Sends an event to every open event stream.
    void push(const QJsonObject &event)
    {
        for (QTcpSocket *stream : std::as_const(m_streams))
            stream->write("data: " + QJsonDocument(event).toJson(QJsonDocument::Compact) + "\n\n");
    }

    // "METHOD /path" of every request, and the body of the last one
    QStringList requests;
    QJsonObject lastBody;
    // answered after this many milliseconds
    int messageDelayMs = 0;
    bool failRename = false;

private:
    void read(QTcpSocket *client)
    {
        QByteArray &buffer = m_buffers[client];
        buffer += client->readAll();
        const qsizetype end = buffer.indexOf("\r\n\r\n");
        if (end == -1)
            return;
        const QList<QByteArray> lines = buffer.left(end).split('\n');
        QHash<QByteArray, QByteArray> headers;
        for (const QByteArray &line : lines.mid(1)) {
            const qsizetype colon = line.indexOf(':');
            if (colon > 0)
                headers.insert(line.left(colon).trimmed().toLower(), line.mid(colon + 1).trimmed());
        }
        const qsizetype length = headers.value("content-length").toLongLong();
        if (buffer.size() < end + 4 + length)
            return;
        const QList<QByteArray> start = lines.first().trimmed().split(' ');
        const QString method = QString::fromUtf8(start.value(0));
        const QString path = QString::fromUtf8(start.value(1)).section(QLatin1Char('?'), 0, 0);
        lastBody = QJsonDocument::fromJson(buffer.mid(end + 4, length)).object();
        buffer.clear();
        requests << method + QLatin1Char(' ') + path;

        auto reply = [client](int status, const QJsonValue &body) {
            const QByteArray json = body.isArray() ? QJsonDocument(body.toArray()).toJson(QJsonDocument::Compact)
                                                   : QJsonDocument(body.toObject()).toJson(QJsonDocument::Compact);
            client->write("HTTP/1.1 " + QByteArray::number(status) + " X\r\nContent-Type: application/json\r\nContent-Length: "
                          + QByteArray::number(json.size()) + "\r\nConnection: close\r\n\r\n" + json);
            client->disconnectFromHost();
        };
        if (headers.value("authorization") != "Basic " + QByteArrayLiteral("opencode:pw").toBase64()) {
            reply(401, QJsonObject{{"message", "unauthorized"}});
            return;
        }
        const QString get = method == QStringLiteral("GET") ? path : QString();
        if (get == "/event") {
            client->write("HTTP/1.1 200 X\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n");
            m_streams << client;
            connect(client, &QObject::destroyed, this, [this, client] { m_streams.removeAll(client); });
        } else if (get == "/global/health") {
            reply(200, QJsonObject{{"healthy", true}, {"version", "1.18.35"}});
        } else if (get == "/config/providers") {
            reply(200, QJsonObject{{"providers", QJsonArray{QJsonObject{
                                                     {"id", "psi"},
                                                     {"name", "PSI"},
                                                     {"models", QJsonObject{{"qwen", QJsonObject{{"name", "Qwen"}, {"limit", QJsonObject{{"context", 262144}}}}}}}}}},
                                   {"default", QJsonObject{{"psi", "qwen"}}}});
        } else if (get == "/config") {
            reply(200, QJsonObject{{"default_agent", "plan"}});
        } else if (get == "/agent") {
            reply(200, QJsonArray{QJsonObject{{"name", "plan"}, {"mode", "primary"}}});
        } else if (get == "/mcp") {
            reply(200, QJsonObject{{"bec", QJsonObject{{"status", "connected"}}}});
        } else if (get == "/session") {
            reply(200, QJsonArray{QJsonObject{{"id", "ses_1"}, {"title", "First"}, {"time", QJsonObject{{"updated", 1}}}}});
        } else if (get == "/session/status" || get == "/permission" || get == "/question") {
            reply(200, get == "/session/status" ? QJsonValue(QJsonObject()) : QJsonValue(QJsonArray()));
        } else if (get.endsWith("/message")) {
            const QJsonArray messages{QJsonObject{{"info", QJsonObject{{"id", "msg_1"}, {"role", "user"}}},
                                                  {"parts", QJsonArray{QJsonObject{{"type", "text"}, {"text", "hello"}}}}}};
            QTimer::singleShot(messageDelayMs, client, [reply, messages] { reply(200, messages); });
        } else if (get.endsWith("/todo")) {
            reply(200, QJsonArray());
        } else if (method == "POST" && path == "/session") {
            reply(200, QJsonObject{{"id", "ses_new"}, {"title", "New"}});
        } else if (method == "POST" && path.endsWith("/prompt_async")) {
            reply(204, QJsonObject());
        } else if (method == "PATCH" && path.startsWith("/session/")) {
            if (failRename)
                reply(500, QJsonObject{{"message", "rename failed"}});
            else
                reply(200, QJsonObject{{"id", path.section(QLatin1Char('/'), 2)}, {"title", lastBody.value("title")}});
        } else {
            reply(404, QJsonObject{{"message", "no such route"}});
        }
    }

    QHash<QTcpSocket *, QByteArray> m_buffers;
    QList<QTcpSocket *> m_streams;
    // Last, so it goes first: its sockets die with it, and a stream's destroyed() still finds
    // m_streams.
    QTcpServer m_server;
};

// Opens the client on the fake and waits until it is open and its event stream listens.
bool openOn(ChatClient &client, FakeOpencode &server)
{
    QSignalSpy opened(&client, &ChatClient::opened);
    client.open(server.connection());
    if (!opened.wait(5000))
        return false;
    QElapsedTimer waited;
    waited.start();
    while (client.stream() != QStringLiteral("open") && waited.elapsed() < 5000)
        QTest::qWait(20);
    return client.stream() == QStringLiteral("open");
}

}  // namespace

class TestChatClient : public QObject
{
    Q_OBJECT

private slots:
    void opensAndLoadsWhatTheServerKnows()
    {
        FakeOpencode server;
        ChatClient client;
        QVERIFY(openOn(client, server));
        QCOMPARE(client.serverVersion(), QStringLiteral("1.18.35"));
        QCOMPARE(client.defaultModel().key(), QStringLiteral("psi/qwen"));
        QCOMPARE(client.model({"psi", "qwen"}).value("limit").toObject().value("context").toInt(), 262144);
        QCOMPARE(client.defaultAgent(), QStringLiteral("plan"));
        QCOMPARE(client.agents().size(), 1);
        QCOMPARE(client.mcpStatus().value("bec").toObject().value("status").toString(), QStringLiteral("connected"));
        QCOMPARE(client.data().sessions.keys(), QStringList{QStringLiteral("ses_1")});
        QVERIFY(client.isOpen());
    }

    void refusesAWrongPassword()
    {
        FakeOpencode server;
        ChatClient client;
        QSignalSpy failed(&client, &ChatClient::openFailed);
        client.open(server.connection(QStringLiteral("wrong")));
        QVERIFY(failed.wait(5000));
        QVERIFY(!client.isOpen());
        QVERIFY(client.connection().isEmpty());
    }

    void foldsEventsIntoTheSessions()
    {
        FakeOpencode server;
        ChatClient client;
        QVERIFY(openOn(client, server));
        QSignalSpy changed(&client, &ChatClient::changed);
        QSignalSpy deleted(&client, &ChatClient::sessionDeleted);

        server.push({{"type", "session.updated"}, {"properties", QJsonObject{{"info", QJsonObject{{"id", "ses_2"}, {"title", "Second"}}}}}});
        QVERIFY(changed.wait(5000));
        QCOMPARE(changed.last().at(0).toString(), QStringLiteral("ses_2"));
        QVERIFY(changed.last().at(1).toInt() & EventEffect::Sessions);
        QVERIFY(client.data().sessions.contains("ses_2"));

        server.push({{"type", "session.deleted"}, {"properties", QJsonObject{{"info", QJsonObject{{"id", "ses_2"}}}}}});
        QVERIFY(deleted.wait(5000));
        QCOMPARE(deleted.last().at(0).toString(), QStringLiteral("ses_2"));
        QVERIFY(!client.data().sessions.contains("ses_2"));
    }

    void loadsTheMessagesOfASession()
    {
        FakeOpencode server;
        ChatClient client;
        QVERIFY(openOn(client, server));
        client.setFocus("ses_1");
        client.loadMessages("ses_1");
        QVERIFY(client.isLoading("ses_1"));
        QTRY_VERIFY_WITH_TIMEOUT(!client.isLoading("ses_1"), 5000);
        QCOMPARE(client.data().messages.value("ses_1").size(), 1);
        QCOMPARE(client.data().messages.value("ses_1").first().id(), QStringLiteral("msg_1"));
    }

    void sendsAPromptToANewSession()
    {
        FakeOpencode server;
        ChatClient client;
        QVERIFY(openOn(client, server));
        QString created;
        QVERIFY(client.createSession([&created](const QString &id, const std::optional<ApiError> &error) {
            QVERIFY(!error);
            created = id;
        }));
        QTRY_COMPARE_WITH_TIMEOUT(created, QStringLiteral("ses_new"), 5000);
        QVERIFY(client.data().sessions.contains("ses_new"));

        bool sent = false;
        QVERIFY(client.prompt("ses_new", "hello", {"psi", "qwen"}, "plan", QString(), [&sent](const std::optional<ApiError> &error) {
            QVERIFY(!error);
            sent = true;
        }));
        QTRY_VERIFY_WITH_TIMEOUT(sent, 5000);
        QCOMPARE(server.requests.last(), QStringLiteral("POST /session/ses_new/prompt_async"));
        QCOMPARE(server.lastBody.value("agent").toString(), QStringLiteral("plan"));
        QCOMPARE(server.lastBody.value("model").toObject().value("modelID").toString(), QStringLiteral("qwen"));
    }

    void reportsAFailedRequestAndStaysOpen()
    {
        FakeOpencode server;
        server.failRename = true;
        ChatClient client;
        QVERIFY(openOn(client, server));
        std::optional<ApiError> failure;
        bool done = false;
        client.renameSession("ses_1", "Renamed", [&](const std::optional<ApiError> &error) {
            failure = error;
            done = true;
        });
        QTRY_VERIFY_WITH_TIMEOUT(done, 5000);
        QVERIFY(failure);
        QCOMPARE(failure->status, 500);
        QVERIFY(client.isOpen());
        QCOMPARE(client.data().sessions.value("ses_1").value("title").toString(), QStringLiteral("First"));
    }

    void dropsAnswersOfAClosedConnection()
    {
        FakeOpencode server;
        server.messageDelayMs = 200;
        ChatClient client;
        QVERIFY(openOn(client, server));
        client.loadMessages("ses_1");
        client.close();
        QSignalSpy changed(&client, &ChatClient::changed);
        QTest::qWait(500);
        QCOMPARE(changed.count(), 0);
        QVERIFY(client.data().messages.isEmpty());
        QVERIFY(!client.renameSession("ses_1", "x", [](const std::optional<ApiError> &) { QFAIL("closed"); }));
    }
};

QTEST_GUILESS_MAIN(TestChatClient)
#include "tst_chatclient.moc"
