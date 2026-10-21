#include "mcpclient.h"

#include "eventstream.h"

#include <QCoreApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QNetworkProxy>
#include <QNetworkReply>
#include <QTimer>

#include <memory>

namespace {

const QString protocolVersion = QStringLiteral("2025-06-18");
const int defaultTimeoutMs = 120000;
const int maxTimeoutMs = 30 * 60 * 1000;

}  // namespace

std::optional<QJsonObject> findMcpResponse(const QByteArray &body, const QString &contentType, qint64 id)
{
    QList<QJsonValue> messages;
    if (contentType.contains(QStringLiteral("text/event-stream"))) {
        SseParser parser;
        for (const QString &data : parser.push(body + "\n\n"))
            messages << QJsonDocument::fromJson(data.toUtf8()).object();
    } else {
        const QJsonDocument document = QJsonDocument::fromJson(body);
        if (document.isArray()) {
            for (const QJsonValue &value : document.array())
                messages << value;
        } else if (document.isObject()) {
            messages << document.object();
        }
    }
    for (const QJsonValue &message : messages) {
        const QJsonValue messageId = message.toObject().value(QStringLiteral("id"));
        if (messageId.isDouble() && qint64(messageId.toDouble()) == id)
            return message.toObject();
    }
    return std::nullopt;
}

QJsonValue mcpToolOutput(const QString &tool, const QJsonObject &result, QString *error)
{
    QString text;
    for (const QJsonValue &content : result.value(QStringLiteral("content")).toArray()) {
        if (content.toObject().value(QStringLiteral("type")).toString() == QStringLiteral("text")) {
            text = content.toObject().value(QStringLiteral("text")).toString();
            break;
        }
    }
    if (result.value(QStringLiteral("isError")).toBool()) {
        *error = text.isEmpty() ? QStringLiteral("%1 failed").arg(tool) : text;
        return {};
    }
    if (result.contains(QStringLiteral("structuredContent")))
        return result.value(QStringLiteral("structuredContent"));
    QJsonParseError parse;
    const QJsonDocument document = QJsonDocument::fromJson(text.toUtf8(), &parse);
    if (parse.error == QJsonParseError::NoError)
        return document.isObject() ? QJsonValue(document.object()) : QJsonValue(document.array());
    return text;
}

McpClient::McpClient(const QUrl &url, const QString &token, QObject *parent)
    : QObject(parent)
    , m_url(url)
    , m_token(token)
{
    m_network.setProxy(QNetworkProxy::NoProxy);
    m_network.setCookieJar(nullptr);
}

std::optional<QUrl> McpClient::loopbackTarget(const QString &raw, QString *error)
{
    const QUrl url(raw, QUrl::StrictMode);
    const QString host = url.host();
    if (!url.isValid() || url.scheme() != QStringLiteral("http")
        || (host != QStringLiteral("127.0.0.1") && host != QStringLiteral("localhost") && host != QStringLiteral("::1"))) {
        if (error)
            *error = QStringLiteral("only http:// MCP servers on 127.0.0.1, localhost or [::1] can be called");
        return std::nullopt;
    }
    if (!url.userName().isEmpty() || !url.password().isEmpty()) {
        if (error)
            *error = QStringLiteral("MCP server URLs with credentials are not called");
        return std::nullopt;
    }
    return url;
}

void McpClient::tools(Callback done)
{
    request({QStringLiteral("tools/list"), {}, defaultTimeoutMs, [done](const QJsonValue &result, const QString &error) {
                 done(error.isEmpty() ? result.toObject().value(QStringLiteral("tools")) : QJsonValue(), error);
             }});
}

void McpClient::call(const QString &tool, const QJsonObject &arguments, int timeoutMs, Callback done)
{
    const int timeout = timeoutMs > 0 ? qMin(timeoutMs, maxTimeoutMs) : defaultTimeoutMs;
    request({QStringLiteral("tools/call"),
             {{QStringLiteral("name"), tool}, {QStringLiteral("arguments"), arguments}},
             timeout,
             [tool, done](const QJsonValue &result, const QString &error) {
                 if (!error.isEmpty()) {
                     done({}, error);
                     return;
                 }
                 QString failed;
                 const QJsonValue output = mcpToolOutput(tool, result.toObject(), &failed);
                 done(output, failed);
             }});
}

void McpClient::initialize(Callback done)
{
    m_session.reset();
    m_initialized << done;
    if (!m_initializing)
        startSession();
}

void McpClient::request(Pending pending)
{
    m_waiting << pending;
    if (m_session)
        flush();
    else if (!m_initializing)
        startSession();
}

void McpClient::flush()
{
    const QList<Pending> waiting = m_waiting;
    m_waiting.clear();
    const QString session = m_session.value_or(QString());
    for (const Pending &pending : waiting) {
        send(pending.method, pending.params, pending.timeoutMs, false, true,
             [this, pending, session](int status, const QJsonValue &result, const QString &error) {
                 if (status == 404 && !session.isEmpty()) {
                     // the server no longer knows the session: start a new one, once
                     if (pending.retried) {
                         pending.done({}, QStringLiteral("%1: session expired").arg(pending.method));
                         return;
                     }
                     if (m_session == session)
                         m_session.reset();
                     Pending again = pending;
                     again.retried = true;
                     request(again);
                     return;
                 }
                 pending.done(result, error);
             });
    }
}

void McpClient::startSession()
{
    m_initializing = true;
    const QJsonObject params{
        {QStringLiteral("protocolVersion"), protocolVersion},
        {QStringLiteral("capabilities"), QJsonObject()},
        {QStringLiteral("clientInfo"),
         QJsonObject{{QStringLiteral("name"), QStringLiteral("bob-gui")},
                     {QStringLiteral("version"), QCoreApplication::applicationVersion()}}},
    };
    auto failed = [this](const QString &error) {
        m_initializing = false;
        const QList<Pending> waiting = m_waiting;
        const QList<Callback> initialized = m_initialized;
        m_waiting.clear();
        m_initialized.clear();
        for (const Pending &pending : waiting)
            pending.done({}, error);
        for (const Callback &done : initialized)
            done({}, error);
    };
    send(QStringLiteral("initialize"), params, defaultTimeoutMs, false, false,
         [this, failed](int status, const QJsonValue &result, const QString &error) {
             if (!error.isEmpty() || status >= 400) {
                 failed(error.isEmpty() ? QStringLiteral("the server refused the session") : error);
                 return;
             }
             m_serverInfo = result.toObject().value(QStringLiteral("serverInfo")).toObject();
             send(QStringLiteral("notifications/initialized"), {}, defaultTimeoutMs, true, true,
                  [this, failed](int, const QJsonValue &, const QString &error) {
                      if (!error.isEmpty()) {
                          failed(error);
                          return;
                      }
                      m_initializing = false;
                      if (!m_session)
                          m_session = QString();
                      const QList<Callback> initialized = m_initialized;
                      m_initialized.clear();
                      for (const Callback &done : initialized)
                          done(m_serverInfo, {});
                      flush();
                  });
         });
}

void McpClient::send(const QString &method, const QJsonObject &params, int timeoutMs, bool notify, bool withSession,
                     std::function<void(int, const QJsonValue &, const QString &)> done)
{
    const qint64 id = m_nextId++;
    QJsonObject message{{QStringLiteral("jsonrpc"), QStringLiteral("2.0")}, {QStringLiteral("method"), method},
                        {QStringLiteral("params"), params}};
    if (!notify)
        message.insert(QStringLiteral("id"), id);
    QNetworkRequest request(m_url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, QStringLiteral("application/json"));
    request.setRawHeader("Accept", "application/json, text/event-stream");
    request.setRawHeader("MCP-Protocol-Version", protocolVersion.toUtf8());
    if (withSession && m_session && !m_session->isEmpty())
        request.setRawHeader("Mcp-Session-Id", m_session->toUtf8());
    if (!m_token.isEmpty())
        request.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());
    request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    request.setAttribute(QNetworkRequest::CookieLoadControlAttribute, QNetworkRequest::Manual);
    request.setAttribute(QNetworkRequest::CookieSaveControlAttribute, QNetworkRequest::Manual);
    QNetworkReply *reply = m_network.post(request, QJsonDocument(message).toJson(QJsonDocument::Compact));
    // A deadline for the whole call: a transfer timeout would only count the time without data,
    // and a server that keeps streaming would never time out.
    auto timedOut = std::make_shared<bool>(false);
    auto *deadline = new QTimer(reply);
    deadline->setSingleShot(true);
    connect(deadline, &QTimer::timeout, reply, [reply, timedOut] {
        *timedOut = true;
        reply->abort();
    });
    deadline->start(timeoutMs);
    connect(reply, &QNetworkReply::finished, this, [this, reply, method, notify, id, done, timedOut, timeoutMs] {
        reply->deleteLater();
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const QByteArray body = reply->readAll();
        if (*timedOut) {
            done(0, {}, QStringLiteral("%1: no answer from %2 within %3 s")
                            .arg(method, m_url.toString(QUrl::RemovePath | QUrl::RemoveQuery))
                            .arg(timeoutMs / 1000));
            return;
        }
        if (status == 0) {
            done(0, {}, QStringLiteral("%1: cannot reach %2: %3")
                            .arg(method, m_url.toString(QUrl::RemovePath | QUrl::RemoveQuery), reply->errorString()));
            return;
        }
        const QByteArray session = reply->rawHeader("Mcp-Session-Id");
        if (!session.isEmpty() && method == QStringLiteral("initialize"))
            m_session = QString::fromUtf8(session);
        if (status == 404) {
            done(status, {}, QStringLiteral("%1: HTTP 404").arg(method));
            return;
        }
        if (status >= 400) {
            const QString detail = QString::fromUtf8(body).trimmed().left(300);
            done(status, {}, detail.isEmpty() ? QStringLiteral("%1: HTTP %2").arg(method).arg(status)
                                              : QStringLiteral("%1: HTTP %2 (%3)").arg(method).arg(status).arg(detail));
            return;
        }
        if (notify) {
            done(status, {}, {});
            return;
        }
        const auto response = findMcpResponse(body, reply->header(QNetworkRequest::ContentTypeHeader).toString(), id);
        if (!response) {
            done(status, {}, QStringLiteral("%1: the server sent no answer").arg(method));
            return;
        }
        if (response->contains(QStringLiteral("error"))) {
            const QString text = response->value(QStringLiteral("error")).toObject().value(QStringLiteral("message")).toString(QStringLiteral("error"));
            done(status, {}, QStringLiteral("%1: %2").arg(method, text));
            return;
        }
        done(status, response->value(QStringLiteral("result")), {});
    });
}
