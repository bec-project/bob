#include "opencodeapi.h"

#include <QHostAddress>
#include <QJsonDocument>
#include <QNetworkProxyFactory>
#include <QNetworkReply>

namespace {

// bob's sandbox is usually on this machine: never send that through a proxy from the
// environment. Anything else uses the system's proxy settings.
class ProxyFactory : public QNetworkProxyFactory
{
public:
    QList<QNetworkProxy> queryProxy(const QNetworkProxyQuery &query) override
    {
        const QString host = query.peerHostName();
        const QHostAddress address(host);
        if (host == QStringLiteral("localhost") || (!address.isNull() && address.isLoopback()))
            return {QNetworkProxy::NoProxy};
        return systemProxyForQuery(query);
    }
};

QString segment(const QString &id)
{
    return QString::fromUtf8(QUrl::toPercentEncoding(id));
}

}  // namespace

QString baseUrl(const QString &url)
{
    QString base = url.trimmed();
    while (base.endsWith(QLatin1Char('/')))
        base.chop(1);
    return base;
}

OpencodeApi::OpencodeApi(const QJsonObject &connection, QObject *parent)
    : QObject(parent)
    , m_base(baseUrl(connection.value(QStringLiteral("url")).toString()))
{
    const QString username = connection.value(QStringLiteral("username")).toString(QStringLiteral("opencode"));
    const QString password = connection.value(QStringLiteral("password")).toString();
    m_auth = "Basic " + (username + QLatin1Char(':') + password).toUtf8().toBase64();
    m_network.setProxyFactory(new ProxyFactory);
}

QNetworkRequest OpencodeApi::newRequest(const QString &path, const QUrlQuery &query) const
{
    QUrl url(m_base + path);
    if (!query.isEmpty())
        url.setQuery(query);
    QNetworkRequest request(url);
    request.setRawHeader("Authorization", m_auth);
    request.setRawHeader("Accept", "application/json");
    return request;
}

void OpencodeApi::request(const QByteArray &method, const QString &path, const QUrlQuery &query,
                          const QJsonValue &body, ApiCallback done)
{
    QNetworkRequest request = newRequest(path, query);
    QByteArray payload;
    if (!body.isUndefined()) {
        request.setHeader(QNetworkRequest::ContentTypeHeader, QStringLiteral("application/json"));
        payload = body.isObject() ? QJsonDocument(body.toObject()).toJson(QJsonDocument::Compact)
                                  : QJsonDocument(body.toArray()).toJson(QJsonDocument::Compact);
    }
    QNetworkReply *reply = m_network.sendCustomRequest(request, method, payload);
    connect(reply, &QNetworkReply::finished, this, [this, reply, method, path, done] {
        reply->deleteLater();
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const QByteArray data = reply->readAll();
        if (status == 0) {
            done({}, ApiError{0, QStringLiteral("Cannot reach %1: %2").arg(m_base, reply->errorString())});
            return;
        }
        if (status < 200 || status >= 300) {
            QString message;
            if (status == 401) {
                message = QStringLiteral("The server refused the password. Paste the connection bob printed for this launch.");
            } else {
                const QString text = QString::fromUtf8(data).left(300);
                message = QStringLiteral("%1 %2 failed (%3)%4")
                              .arg(QString::fromLatin1(method), path)
                              .arg(status)
                              .arg(text.isEmpty() ? QString() : QStringLiteral(": ") + text);
            }
            done({}, ApiError{status, message});
            return;
        }
        if (status == 204 || data.isEmpty()) {
            done(QJsonValue::Null, std::nullopt);
            return;
        }
        const QString type = reply->header(QNetworkRequest::ContentTypeHeader).toString();
        if (!type.contains(QStringLiteral("json"))) {
            done(QJsonValue(QString::fromUtf8(data)), std::nullopt);
            return;
        }
        QJsonParseError error;
        const QJsonDocument document = QJsonDocument::fromJson(data, &error);
        if (error.error != QJsonParseError::NoError) {
            // OpenCode answers some calls with a bare JSON value (true, a string)
            const QJsonDocument wrapped = QJsonDocument::fromJson("[" + data + "]");
            done(wrapped.isArray() ? wrapped.array().first() : QJsonValue(QString::fromUtf8(data)), std::nullopt);
            return;
        }
        done(document.isObject() ? QJsonValue(document.object()) : QJsonValue(document.array()), std::nullopt);
    });
}

void OpencodeApi::health(ApiCallback done)
{
    request("GET", QStringLiteral("/global/health"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::sessions(ApiCallback done)
{
    QUrlQuery query;
    query.addQueryItem(QStringLiteral("roots"), QStringLiteral("true"));
    request("GET", QStringLiteral("/session"), query, QJsonValue::Undefined, done);
}

void OpencodeApi::createSession(ApiCallback done)
{
    request("POST", QStringLiteral("/session"), {}, QJsonObject(), done);
}

void OpencodeApi::renameSession(const QString &id, const QString &title, ApiCallback done)
{
    request("PATCH", QStringLiteral("/session/%1").arg(segment(id)), {}, QJsonObject{{QStringLiteral("title"), title}}, done);
}

void OpencodeApi::deleteSession(const QString &id, ApiCallback done)
{
    request("DELETE", QStringLiteral("/session/%1").arg(segment(id)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::sessionStatus(ApiCallback done)
{
    request("GET", QStringLiteral("/session/status"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::messages(const QString &sessionID, ApiCallback done)
{
    request("GET", QStringLiteral("/session/%1/message").arg(segment(sessionID)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::prompt(const QString &sessionID, const QString &text, const ModelRef &model, const QString &agent,
                         const QString &variant, ApiCallback done)
{
    QJsonObject body{
        {QStringLiteral("parts"),
         QJsonArray{QJsonObject{{QStringLiteral("type"), QStringLiteral("text")}, {QStringLiteral("text"), text}}}},
    };
    if (model.isValid()) {
        body.insert(QStringLiteral("model"), QJsonObject{{QStringLiteral("providerID"), model.providerID},
                                                         {QStringLiteral("modelID"), model.modelID}});
    }
    if (!agent.isEmpty())
        body.insert(QStringLiteral("agent"), agent);
    if (!variant.isEmpty())
        body.insert(QStringLiteral("variant"), variant);
    request("POST", QStringLiteral("/session/%1/prompt_async").arg(segment(sessionID)), {}, body, done);
}

void OpencodeApi::compact(const QString &sessionID, const ModelRef &model, ApiCallback done)
{
    request("POST", QStringLiteral("/session/%1/summarize").arg(segment(sessionID)), {},
            QJsonObject{{QStringLiteral("providerID"), model.providerID}, {QStringLiteral("modelID"), model.modelID}}, done);
}

void OpencodeApi::diff(const QString &sessionID, const QString &messageID, ApiCallback done)
{
    QUrlQuery query;
    if (!messageID.isEmpty())
        query.addQueryItem(QStringLiteral("messageID"), messageID);
    request("GET", QStringLiteral("/session/%1/diff").arg(segment(sessionID)), query, QJsonValue::Undefined, done);
}

void OpencodeApi::revert(const QString &sessionID, const QString &messageID, ApiCallback done)
{
    request("POST", QStringLiteral("/session/%1/revert").arg(segment(sessionID)), {},
            QJsonObject{{QStringLiteral("messageID"), messageID}}, done);
}

void OpencodeApi::unrevert(const QString &sessionID, ApiCallback done)
{
    request("POST", QStringLiteral("/session/%1/unrevert").arg(segment(sessionID)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::fork(const QString &sessionID, const QString &messageID, ApiCallback done)
{
    QJsonObject body;
    if (!messageID.isEmpty())
        body.insert(QStringLiteral("messageID"), messageID);
    request("POST", QStringLiteral("/session/%1/fork").arg(segment(sessionID)), {}, body, done);
}

void OpencodeApi::todos(const QString &sessionID, ApiCallback done)
{
    request("GET", QStringLiteral("/session/%1/todo").arg(segment(sessionID)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::abort(const QString &sessionID, ApiCallback done)
{
    request("POST", QStringLiteral("/session/%1/abort").arg(segment(sessionID)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::providers(ApiCallback done)
{
    request("GET", QStringLiteral("/config/providers"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::config(ApiCallback done)
{
    request("GET", QStringLiteral("/config"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::agents(ApiCallback done)
{
    request("GET", QStringLiteral("/agent"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::mcpStatus(ApiCallback done)
{
    request("GET", QStringLiteral("/mcp"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::mcpConnect(const QString &name, ApiCallback done)
{
    request("POST", QStringLiteral("/mcp/%1/connect").arg(segment(name)), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::permissions(ApiCallback done)
{
    request("GET", QStringLiteral("/permission"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::replyPermission(const QString &requestID, const QString &reply, ApiCallback done)
{
    request("POST", QStringLiteral("/permission/%1/reply").arg(segment(requestID)), {},
            QJsonObject{{QStringLiteral("reply"), reply}}, done);
}

void OpencodeApi::questions(ApiCallback done)
{
    request("GET", QStringLiteral("/question"), {}, QJsonValue::Undefined, done);
}

void OpencodeApi::replyQuestion(const QString &requestID, const QJsonArray &answers, ApiCallback done)
{
    request("POST", QStringLiteral("/question/%1/reply").arg(segment(requestID)), {},
            QJsonObject{{QStringLiteral("answers"), answers}}, done);
}

void OpencodeApi::rejectQuestion(const QString &requestID, ApiCallback done)
{
    request("POST", QStringLiteral("/question/%1/reject").arg(segment(requestID)), {}, QJsonValue::Undefined, done);
}
