#pragma once

#include "opencodeapi.h"
#include "sessiondata.h"

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QTimer>

#include <functional>
#include <optional>

class EventStream;

// One connection to bob's OpenCode server: the sessions the event stream keeps current, OpenCode's
// models, agents and MCP servers, and the requests that change them. It knows nothing of how they
// are shown; `changed` says which session and which part of it to show again.
class ChatClient : public QObject
{
    Q_OBJECT

public:
    using Done = std::function<void(const std::optional<ApiError> &error)>;
    // A request that makes a session: its id, or the error
    using Created = std::function<void(const QString &sessionID, const std::optional<ApiError> &error)>;

    explicit ChatClient(QObject *parent = nullptr);
    ~ChatClient() override;

    // Connects to the OpenCode server of this connection (url, password, ...) and loads what it
    // knows; `opened` or `openFailed` says how that ended, then the event stream keeps it current.
    void open(const QJsonObject &connection);
    // Stops talking to the server and forgets everything it said.
    void close();

    // A connection is open or being opened.
    bool isOpen() const { return m_api; }
    QJsonObject connection() const { return m_connection; }
    QString serverVersion() const { return m_serverVersion; }
    // connecting, open, retrying or closed
    QString stream() const { return m_stream; }
    QString streamDetail() const { return m_streamDetail; }
    const SessionData &data() const { return m_data; }
    bool isLoading(const QString &sessionID) const { return m_loading.value(sessionID); }
    const QJsonArray &providers() const { return m_providers; }
    ModelRef defaultModel() const { return m_defaultModel; }
    const QJsonArray &agents() const { return m_agents; }
    QString defaultAgent() const { return m_defaultAgent; }
    // OpenCode's status of each MCP server, by name
    const QJsonObject &mcpStatus() const { return m_mcpStatus; }
    // What OpenCode knows of a model (limits, variants), or {}
    QJsonObject model(const ModelRef &ref) const;

    // The session whose todos and workspace changes are kept loaded: the one on screen.
    void setFocus(const QString &sessionID) { m_focus = sessionID; }
    void loadMessages(const QString &sessionID);
    // Its todos and changes.
    void loadExtras(const QString &sessionID);

    // The requests return false, and never call `done`, while there is no connection.
    bool createSession(Created done);
    bool prompt(const QString &sessionID, const QString &text, const ModelRef &model, const QString &agent,
                const QString &variant, Done done);
    bool abort(const QString &sessionID, Done done);
    bool renameSession(const QString &sessionID, const QString &title, Done done);
    bool deleteSession(const QString &sessionID, Done done);
    // reply is once, always or reject
    bool replyPermission(const QString &id, const QString &reply, Done done);
    // One list of chosen labels per question
    bool replyQuestion(const QString &id, const QJsonArray &answers, Done done);
    bool rejectQuestion(const QString &id, Done done);
    bool compact(const QString &sessionID, const ModelRef &model, Done done);
    bool revert(const QString &sessionID, const QString &messageID, Done done);
    bool unrevert(const QString &sessionID, Done done);
    // Copies a session up to (not including) messageID into a new one.
    bool fork(const QString &sessionID, const QString &messageID, Created done);
    // Asks OpenCode to connect to an MCP server again, then reloads every server's status.
    bool reconnectMcp(const QString &name, Done done);

signals:
    // Connected: models, agents, MCP servers and the session list are loaded.
    void opened();
    void openFailed(const QString &error);
    // The server no longer accepts the password; the connection is closed.
    void unauthorized();
    // Parts of a session changed (EventEffect::Area bits); an empty id stands for every session.
    void changed(const QString &sessionID, int areas);
    void sessionDeleted(const QString &sessionID);
    void mcpChanged();
    void streamChanged();
    // A load the client started by itself failed.
    void failed(const ApiError &error);

private:
    void stop();
    void onEvent(const QJsonObject &event);
    void loadChanges(const QString &sessionID);
    void resync(const QString &sessionID);
    void refreshMcp();
    void refreshLists(std::function<void(bool ok, QString error)> done);
    // Runs `done` for a request of this connection only, never for one a reconnect replaced.
    ApiCallback guarded(std::function<void(const QJsonValue &, const std::optional<ApiError> &)> done);
    bool current(const OpencodeApi *api) const { return api && api == m_api; }

    QPointer<OpencodeApi> m_api;
    QPointer<EventStream> m_events;
    QJsonObject m_connection;
    QString m_serverVersion;
    QString m_stream = QStringLiteral("closed");
    QString m_streamDetail;

    SessionData m_data;
    QHash<QString, bool> m_loading;
    QString m_focus;
    QJsonArray m_providers;
    ModelRef m_defaultModel;
    QJsonArray m_agents;
    QString m_defaultAgent = QStringLiteral("build");
    QJsonObject m_mcpStatus;

    QTimer m_mcpTimer;
    QHash<QString, QTimer *> m_resync;
};
