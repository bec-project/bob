#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QJsonValue>
#include <QNetworkAccessManager>
#include <QObject>
#include <QUrlQuery>

#include <functional>
#include <optional>

struct ApiError
{
    // The HTTP status, or 0 when the server could not be reached
    int status = 0;
    QString message;
};

// The result of a request: its JSON (a string for a non-JSON body, null for none), or the error.
using ApiCallback = std::function<void(const QJsonValue &result, const std::optional<ApiError> &error)>;

struct ModelRef
{
    QString providerID;
    QString modelID;
    bool isValid() const { return !providerID.isEmpty() && !modelID.isEmpty(); }
    QString key() const { return isValid() ? providerID + QLatin1Char('/') + modelID : QString(); }
    // "provider/model", as key() writes it; invalid when it is not one
    static ModelRef fromKey(const QString &key)
    {
        const qsizetype slash = key.indexOf(QLatin1Char('/'));
        if (slash <= 0)
            return {};
        return {key.left(slash), key.mid(slash + 1)};
    }
    bool operator==(const ModelRef &other) const
    {
        return providerID == other.providerID && modelID == other.modelID;
    }
};

// A small client for the OpenCode server API that bob serves. Every request carries
// HTTP Basic auth with bob's generated password. Callbacks never run after the client is gone.
class OpencodeApi : public QObject
{
    Q_OBJECT
public:
    // `connection` is bob's connection object: url and password (username defaults to opencode)
    explicit OpencodeApi(const QJsonObject &connection, QObject *parent = nullptr);

    QString base() const { return m_base; }
    QByteArray authorization() const { return m_auth; }
    QNetworkAccessManager *network() { return &m_network; }
    QNetworkRequest newRequest(const QString &path, const QUrlQuery &query = {}) const;

    void request(const QByteArray &method, const QString &path, const QUrlQuery &query, const QJsonValue &body,
                 ApiCallback done);

    void health(ApiCallback done);
    void sessions(ApiCallback done);
    void createSession(ApiCallback done);
    void renameSession(const QString &id, const QString &title, ApiCallback done);
    void deleteSession(const QString &id, ApiCallback done);
    void sessionStatus(ApiCallback done);
    void messages(const QString &sessionID, ApiCallback done);
    void prompt(const QString &sessionID, const QString &text, const ModelRef &model, const QString &agent,
                const QString &variant, ApiCallback done);
    // Summarises the chat so far and continues from the summary (OpenCode's compaction).
    void compact(const QString &sessionID, const ModelRef &model, ApiCallback done);
    // Files the chat changed in the workspace, or those one message changed.
    void diff(const QString &sessionID, const QString &messageID, ApiCallback done);
    // Undoes a message and everything after it, files included, until the next prompt.
    void revert(const QString &sessionID, const QString &messageID, ApiCallback done);
    void unrevert(const QString &sessionID, ApiCallback done);
    // A new chat with this chat's messages before messageID (all of them without one).
    void fork(const QString &sessionID, const QString &messageID, ApiCallback done);
    void todos(const QString &sessionID, ApiCallback done);
    void abort(const QString &sessionID, ApiCallback done);
    void providers(ApiCallback done);
    void config(ApiCallback done);
    void agents(ApiCallback done);
    void mcpStatus(ApiCallback done);
    // Asks OpenCode to connect to one of its MCP servers again.
    void mcpConnect(const QString &name, ApiCallback done);
    void permissions(ApiCallback done);
    // reply is once, always or reject
    void replyPermission(const QString &requestID, const QString &reply, ApiCallback done);
    void questions(ApiCallback done);
    // One list of chosen labels per question
    void replyQuestion(const QString &requestID, const QJsonArray &answers, ApiCallback done);
    void rejectQuestion(const QString &requestID, ApiCallback done);

private:
    QString m_base;
    QByteArray m_auth;
    QNetworkAccessManager m_network;
};

// Normalises the base URL bob prints (http://127.0.0.1:PORT, maybe with a trailing slash).
QString baseUrl(const QString &url);
