#pragma once

#include <QJsonObject>
#include <QJsonValue>
#include <QNetworkAccessManager>
#include <QObject>
#include <QUrl>

#include <functional>
#include <optional>

// A small MCP client for the plugins' own calls and the servers' versions, over MCP's
// streamable-HTTP transport: initialize once per server, then list and call tools. The model's
// tool calls never pass here; OpenCode makes those itself.
//
// Only plain http:// servers on this machine's loopback interface are reached, without proxies,
// redirects or cookies.
class McpClient : public QObject
{
    Q_OBJECT
public:
    // The result of a request (a tool's output, or tools/list's result), or the error in words.
    using Callback = std::function<void(const QJsonValue &result, const QString &error)>;

    McpClient(const QUrl &url, const QString &token, QObject *parent = nullptr);

    // The server's tools (the first page; the servers bob uses list all of theirs at once).
    void tools(Callback done);
    // Calls a tool and returns its output: its structured content, or its text (as JSON when it
    // parses). `timeoutMs` bounds the whole call; 0 means two minutes.
    void call(const QString &tool, const QJsonObject &arguments, int timeoutMs, Callback done);
    // Starts a new session and returns the server's serverInfo ({name, version, ...}).
    void initialize(Callback done);

    QJsonObject serverInfo() const { return m_serverInfo; }

    // The server's URL when it is plain HTTP on this machine's loopback interface, or nothing.
    static std::optional<QUrl> loopbackTarget(const QString &url, QString *error = nullptr);

private:
    struct Pending
    {
        QString method;
        QJsonObject params;
        int timeoutMs;
        Callback done;
        bool retried = false;
    };

    void request(Pending pending);
    void flush();
    void startSession();
    void send(const QString &method, const QJsonObject &params, int timeoutMs, bool notify, bool withSession,
              std::function<void(int status, const QJsonValue &result, const QString &error)> done);

    QUrl m_url;
    QString m_token;
    QNetworkAccessManager m_network;
    qint64 m_nextId = 1;
    // The mcp-session-id once initialized (it may be empty: a server without sessions)
    std::optional<QString> m_session;
    bool m_initializing = false;
    QList<Pending> m_waiting;
    QList<Callback> m_initialized;
    QJsonObject m_serverInfo;
};

// The JSON-RPC response with this id in a JSON or text/event-stream body.
std::optional<QJsonObject> findMcpResponse(const QByteArray &body, const QString &contentType, qint64 id);
// What a tool call returned: its structured content, or its text (as JSON when it parses); an
// error when the call failed.
QJsonValue mcpToolOutput(const QString &tool, const QJsonObject &result, QString *error);
