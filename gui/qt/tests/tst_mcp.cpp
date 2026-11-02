// The plugins' MCP client, against a fake MCP server on the loopback interface.

#include "mcpclient.h"

#include <QElapsedTimer>
#include <QJsonArray>
#include <QJsonDocument>
#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTest>
#include <QTimer>

namespace {

// Speaks MCP's streamable HTTP just enough: sessions (which it can forget), tools/list as an event
// stream, and tools/call with text, structured or failed results, or a stream that never ends.
class FakeMcpServer : public QObject
{
public:
    FakeMcpServer()
    {
        QVERIFY(m_server.listen(QHostAddress::LocalHost));
        connect(&m_server, &QTcpServer::newConnection, this, [this] {
            while (QTcpSocket *client = m_server.nextPendingConnection())
                connect(client, &QTcpSocket::readyRead, this, [this, client] { read(client); });
        });
    }

    QString url() const { return QStringLiteral("http://127.0.0.1:%1/mcp").arg(m_server.serverPort()); }

    int initializations = 0;
    QStringList methods;
    QString lastToken;
    bool forget = false;

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
        const QJsonObject message = QJsonDocument::fromJson(buffer.mid(end + 4, length)).object();
        buffer.clear();
        lastToken = QString::fromUtf8(headers.value("authorization"));
        const QString method = message.value("method").toString();
        methods << method;
        const QJsonValue id = message.value("id");
        auto reply = [client](int status, const QByteArray &type, const QByteArray &body, const QByteArray &extra = {}) {
            client->write("HTTP/1.1 " + QByteArray::number(status) + " X\r\nContent-Type: " + type + "\r\nContent-Length: "
                          + QByteArray::number(body.size()) + "\r\n" + extra + "Connection: close\r\n\r\n" + body);
            client->disconnectFromHost();
        };
        auto result = [&id](const QJsonObject &value) {
            return QJsonDocument(QJsonObject{{"jsonrpc", "2.0"}, {"id", id}, {"result", value}}).toJson(QJsonDocument::Compact);
        };
        if (method == "initialize") {
            m_session = QStringLiteral("session-%1").arg(++initializations);
            reply(200, "application/json",
                  result({{"protocolVersion", "2025-06-18"}, {"serverInfo", QJsonObject{{"name", "fake"}, {"version", "1.4.0"}}}}),
                  "Mcp-Session-Id: " + m_session.toUtf8() + "\r\n");
            return;
        }
        if (headers.value("mcp-session-id") != m_session.toUtf8() || forget) {
            forget = false;
            m_session.clear();
            reply(404, "text/plain", "unknown session");
            return;
        }
        if (method == "notifications/initialized") {
            reply(202, "text/plain", {});
        } else if (method == "tools/list") {
            reply(200, "text/event-stream",
                  "event: message\r\ndata: " + result({{"tools", QJsonArray{QJsonObject{{"name", "bec_ping"}}}}}) + "\r\n\r\n");
        } else if (method == "tools/call") {
            const QString tool = message.value("params").toObject().value("name").toString();
            if (tool == "trickle") {
                // an event stream that keeps sending comments and never answers
                client->write("HTTP/1.1 200 X\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n");
                auto *ping = new QTimer(client);
                connect(ping, &QTimer::timeout, client, [client] { client->write(": ping\n\n"); });
                ping->start(50);
                return;
            }
            QJsonObject value;
            if (tool == "text")
                value = {{"content", QJsonArray{QJsonObject{{"type", "text"}, {"text", "{\"summary\": \"ok\"}"}}}}};
            else if (tool == "structured")
                value = {{"content", QJsonArray()}, {"structuredContent", QJsonObject{{"summary", "s"}}}};
            else
                value = {{"isError", true}, {"content", QJsonArray{QJsonObject{{"type", "text"}, {"text", "no such device"}}}}};
            reply(200, "application/json", result(value));
        } else {
            reply(400, "text/plain", "what?");
        }
    }

    QTcpServer m_server;
    QHash<QTcpSocket *, QByteArray> m_buffers;
    QString m_session;
};

struct Answer
{
    bool done = false;
    QJsonValue result;
    QString error;
};

McpClient::Callback into(Answer &answer)
{
    return [&answer](const QJsonValue &result, const QString &error) {
        answer.done = true;
        answer.result = result;
        answer.error = error;
    };
}

}  // namespace

class TestMcp : public QObject
{
    Q_OBJECT

private slots:
    void callsOnlyLoopbackServers()
    {
        QVERIFY(McpClient::loopbackTarget("http://127.0.0.1:8000/mcp"));
        QVERIFY(McpClient::loopbackTarget("http://localhost:8000/mcp"));
        QVERIFY(McpClient::loopbackTarget("http://[::1]:8000/mcp"));
        QString error;
        QVERIFY(!McpClient::loopbackTarget("https://127.0.0.1/mcp", &error));
        QVERIFY(!McpClient::loopbackTarget("http://bec.psi.ch/mcp", &error));
        QVERIFY(error.startsWith("only http://"));
        QVERIFY(!McpClient::loopbackTarget("http://user:pw@127.0.0.1/mcp", &error));
    }

    void findsTheResponseInJsonOrAnEventStream()
    {
        const QByteArray json = "[{\"id\":1,\"result\":{}},{\"id\":2,\"result\":{\"a\":1}}]";
        QCOMPARE(findMcpResponse(json, "application/json", 2)->value("result").toObject().value("a").toInt(), 1);
        const QByteArray stream = "event: message\ndata: {\"method\":\"x\"}\n\ndata: {\"id\":7,\n"
                                  "data: \"result\":{}}\n\n";
        QVERIFY(findMcpResponse(stream, "text/event-stream", 7));
        QVERIFY(!findMcpResponse(stream, "text/event-stream", 8));
    }

    void readsAToolsOutput()
    {
        QString error;
        const QJsonObject text{{"content", QJsonArray{QJsonObject{{"type", "text"}, {"text", "{\"summary\":\"s\"}"}}}}};
        QCOMPARE(mcpToolOutput("t", text, &error).toObject().value("summary").toString(), QString("s"));
        const QJsonObject plain{{"content", QJsonArray{QJsonObject{{"type", "text"}, {"text", "hello"}}}}};
        QCOMPARE(mcpToolOutput("t", plain, &error).toString(), QString("hello"));
        const QJsonObject failed{{"isError", true}, {"content", QJsonArray()}};
        mcpToolOutput("bec_ping", failed, &error);
        QCOMPARE(error, QString("bec_ping failed"));
    }

    void initializesOnceThenListsAndCallsTools()
    {
        FakeMcpServer server;
        McpClient client(QUrl(server.url()), "secret");
        Answer tools, call;
        client.tools(into(tools));
        client.call("text", {}, 0, into(call));
        QTRY_VERIFY(tools.done && call.done);
        QCOMPARE(server.initializations, 1);
        QCOMPARE(tools.error, QString());
        QCOMPARE(tools.result.toArray().first().toObject().value("name").toString(), QString("bec_ping"));
        QCOMPARE(call.result.toObject().value("summary").toString(), QString("ok"));
        QCOMPARE(server.lastToken, QString("Bearer secret"));
        QCOMPARE(client.serverInfo().value("version").toString(), QString("1.4.0"));

        Answer structured, failed;
        client.call("structured", {}, 0, into(structured));
        client.call("broken", {}, 0, into(failed));
        QTRY_VERIFY(structured.done && failed.done);
        QCOMPARE(structured.result.toObject().value("summary").toString(), QString("s"));
        QCOMPARE(failed.error, QString("no such device"));
        QCOMPARE(server.initializations, 1);
    }

    void startsANewSessionWhenTheServerForgotIt()
    {
        FakeMcpServer server;
        McpClient client(QUrl(server.url()), {});
        Answer first;
        client.call("text", {}, 0, into(first));
        QTRY_VERIFY(first.done);
        server.forget = true;
        Answer second;
        client.call("text", {}, 0, into(second));
        QTRY_VERIFY(second.done);
        QCOMPARE(second.error, QString());
        QCOMPARE(server.initializations, 2);
    }

    void reportsTheServersVersion()
    {
        FakeMcpServer server;
        McpClient client(QUrl(server.url()), {});
        Answer info;
        client.initialize(into(info));
        QTRY_VERIFY(info.done);
        QCOMPARE(info.result.toObject().value("version").toString(), QString("1.4.0"));
    }

    void givesUpOnACallThatKeepsStreamingPastItsTimeout()
    {
        FakeMcpServer server;
        McpClient client(QUrl(server.url()), {});
        Answer answer;
        QElapsedTimer clock;
        clock.start();
        client.call("trickle", {}, 400, into(answer));
        QTRY_VERIFY_WITH_TIMEOUT(answer.done, 5000);
        QVERIFY2(answer.error.contains("within"), qPrintable(answer.error));
        QVERIFY(clock.elapsed() < 3000);
    }

    void saysWhenTheServerIsNotThere()
    {
        McpClient client(QUrl("http://127.0.0.1:9/mcp"), {});
        Answer answer;
        client.tools(into(answer));
        QTRY_VERIFY(answer.done);
        QVERIFY2(answer.error.contains("cannot reach"), qPrintable(answer.error));
    }
};

QTEST_GUILESS_MAIN(TestMcp)
#include "tst_mcp.moc"
