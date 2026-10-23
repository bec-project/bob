#pragma once

#include "chatlogic.h"
#include "opencodeapi.h"
#include "sessiondata.h"
#include "sessionlistmodel.h"
#include "threadmodel.h"

#include <QHash>
#include <QObject>
#include <QPointer>
#include <QQmlEngine>
#include <QSet>
#include <QTimer>
#include <QVariantList>

class Bob;
class EventStream;
class Prefs;

// The chat with OpenCode: the connection to the server in bob's container, every session the
// event stream keeps current, and what the window shows of them. Talks to OpenCode directly over
// HTTP; Bob holds the lease that keeps the container running.
class Chat : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    // setup (the connect screen), starting (joining bob), connecting (to OpenCode) or ready
    Q_PROPERTY(QString phase READ phase NOTIFY phaseChanged)
    // Why the last connection attempt failed, shown on the connect screen
    Q_PROPERTY(QString connectError READ connectError NOTIFY phaseChanged)
    Q_PROPERTY(QVariantMap connection READ connection NOTIFY phaseChanged)
    Q_PROPERTY(QString serverVersion READ serverVersion NOTIFY phaseChanged)
    // connecting, open, retrying or closed
    Q_PROPERTY(QString stream READ stream NOTIFY streamChanged)
    Q_PROPERTY(QString streamDetail READ streamDetail NOTIFY streamChanged)

    Q_PROPERTY(SessionListModel *sessions READ sessions CONSTANT)
    Q_PROPERTY(ThreadModel *thread READ thread CONSTANT)
    // The open chat; empty for a new one
    Q_PROPERTY(QString currentId READ currentId NOTIFY currentChanged)
    Q_PROPERTY(QString title READ title NOTIFY currentChanged)
    Q_PROPERTY(bool busy READ busy NOTIFY currentChanged)
    Q_PROPERTY(bool hasMessages READ hasMessages NOTIFY currentChanged)
    Q_PROPERTY(bool compacting READ compacting NOTIFY currentChanged)
    // The first message an undo removed, until the next prompt
    Q_PROPERTY(QString revertID READ revertID NOTIFY currentChanged)
    // Something in the open chat runs (it, or a sub-agent): timers tick
    Q_PROPERTY(bool running READ running NOTIFY currentChanged)
    // Totals for the open chat; empty without messages
    Q_PROPERTY(QVariantMap usage READ usage NOTIFY usageChanged)

    // [{key, label}] of every model OpenCode offers
    Q_PROPERTY(QVariantList modelOptions READ modelOptions NOTIFY modelsChanged)
    // The model the next prompt uses: the user's pick while the server offers it
    Q_PROPERTY(QString activeModel READ activeModel NOTIFY modelsChanged)
    Q_PROPERTY(QString activeModelLabel READ activeModelLabel NOTIFY modelsChanged)
    // The active model's variant that switches thinking off, or empty
    Q_PROPERTY(QString thinkingVariant READ thinkingVariant NOTIFY modelsChanged)
    Q_PROPERTY(QStringList agentOptions READ agentOptions NOTIFY modelsChanged)
    Q_PROPERTY(QString activeAgent READ activeAgent NOTIFY modelsChanged)
    // [{name, status, error, tone, text, retry}] for every MCP server OpenCode uses
    Q_PROPERTY(QVariantList mcpServers READ mcpServers NOTIFY mcpChanged)

    // The open chat's permission requests and questions, oldest first
    Q_PROPERTY(QVariantList permissions READ permissions NOTIFY requestsChanged)
    Q_PROPERTY(QVariantList questions READ questions NOTIFY requestsChanged)
    // The agent's todo list for the open chat
    Q_PROPERTY(QVariantList todos READ todos NOTIFY todosChanged)
    // What each prompt of the open chat changed, newest first
    Q_PROPERTY(QVariantList changes READ changes NOTIFY changesChanged)
    Q_PROPERTY(int changedFiles READ changedFiles NOTIFY changesChanged)
    // A message for the user ({text, tone: error|info}), until the next action
    Q_PROPERTY(QVariantMap notice READ notice NOTIFY noticeChanged)
    // Why the broker ended the lease on the bob this window uses; empty while it holds
    Q_PROPERTY(QString lostReason READ lostReason NOTIFY lostChanged)

public:
    Chat(Bob *bob, Prefs *prefs, QObject *parent = nullptr);
    ~Chat() override;

    static Chat *create(QQmlEngine *, QJSEngine *);
    static void setInstance(Chat *chat);

    QString phase() const { return m_phase; }
    QString connectError() const { return m_connectError; }
    QVariantMap connection() const { return m_connection.toVariantMap(); }
    QString serverVersion() const { return m_serverVersion; }
    QString stream() const { return m_stream; }
    QString streamDetail() const { return m_streamDetail; }
    SessionListModel *sessions() const { return m_sessionList; }
    ThreadModel *thread() const { return m_thread; }
    QString currentId() const { return m_currentId; }
    QString title() const;
    bool busy() const { return isBusy(m_currentId); }
    bool hasMessages() const;
    bool compacting() const;
    QString revertID() const;
    bool running() const;
    QVariantMap usage() const { return m_usage; }
    QVariantList modelOptions() const;
    QString activeModel() const { return activeModelRef().key(); }
    QString activeModelLabel() const;
    QString thinkingVariant() const;
    QStringList agentOptions() const;
    QString activeAgent() const;
    QVariantList mcpServers() const;
    QVariantList permissions() const;
    QVariantList questions() const;
    QVariantList todos() const;
    QVariantList changes() const;
    int changedFiles() const;
    QVariantMap notice() const { return m_notice; }
    QString lostReason() const;
    const SessionData &data() const { return m_data; }

    // Connects to the OpenCode server of this connection (url, password, ...).
    Q_INVOKABLE void connectTo(const QVariantMap &connection);
    // Joins the user's bob, starting it with the chosen bob.toml when none runs, then connects.
    Q_INVOKABLE void joinBob();
    // Ends the app's lease on bob, if it holds one, and goes back to the connect screen.
    Q_INVOKABLE void leaveBob();
    Q_INVOKABLE void disconnect();
    // Opens a chat; an empty id starts a new one.
    Q_INVOKABLE void selectSession(const QString &id);
    // Sends a prompt to the open chat, or to a new one. `typed` prompts (from the composer) are
    // remembered for Up in a new chat; suggestions and plugins' prompts are not.
    Q_INVOKABLE void send(const QString &text, bool typed = false);
    Q_INVOKABLE void abort();
    Q_INVOKABLE void renameSession(const QString &id, const QString &title);
    Q_INVOKABLE void deleteSession(const QString &id);
    // reply is once, always or reject
    Q_INVOKABLE void replyPermission(const QString &id, const QString &reply);
    // One list of chosen labels per question
    Q_INVOKABLE void replyQuestion(const QString &id, const QVariantList &answers);
    Q_INVOKABLE void rejectQuestion(const QString &id);
    // Summarises the open chat so the next prompts start from a short context.
    Q_INVOKABLE void compact();
    // Undoes a user message and everything after it, workspace files included.
    Q_INVOKABLE void revert(const QString &messageID);
    Q_INVOKABLE void unrevert();
    // Copies the open chat up to (not including) messageID into a new chat and opens it.
    Q_INVOKABLE void fork(const QString &messageID);
    // Asks OpenCode to connect to an MCP server again, then reloads every server's status.
    Q_INVOKABLE void reconnectMcp(const QString &name);
    Q_INVOKABLE void setNotice(const QString &text);
    Q_INVOKABLE void dismissNotice();
    Q_INVOKABLE void setModel(const QString &key);
    Q_INVOKABLE void setAgent(const QString &name);
    // Opens or closes a row of a thread (a tool call, the thinking, a summary).
    Q_INVOKABLE void setExpanded(const QString &key, bool expanded);
    // A sub-agent's steps, loaded on demand and kept live by the event stream.
    Q_INVOKABLE ThreadModel *stepsModel(const QString &sessionID);
    Q_INVOKABLE void copyChat();
    Q_INVOKABLE void exportChat();
    // Up (`up`) or Down on the composer's edge: {text} to show, or {} to move the caret as usual.
    Q_INVOKABLE QVariantMap recall(bool up, const QString &current);
    // The composer changed by typing: stepping through the history ends.
    Q_INVOKABLE void endRecall();

signals:
    void phaseChanged();
    void streamChanged();
    void currentChanged();
    void usageChanged();
    void modelsChanged();
    void mcpChanged();
    void requestsChanged();
    void todosChanged();
    void changesChanged();
    void noticeChanged();
    void lostChanged();
    // Put text into the composer: replace it, or add it to what is there.
    void composerRequest(const QString &text, bool replace);
    // A reply to the permission request `id` did not reach OpenCode; the request is still open.
    void permissionReplyFailed(const QString &id);

private:
    enum Dirty {
        SessionList = 1,
        Thread = 2,
        Current = 4,
        Usage = 8,
        Requests = 16,
        Todos = 32,
        Changes = 64,
        Children = 128,
    };

    void setPhase(const QString &phase, const QString &error = {});
    void fail(const ApiError &error);
    void touch(int dirty);
    void flush();
    void evaluateJoin();
    void onBobStatus();
    void onEvent(const QJsonObject &event);
    void loadMessages(const QString &sessionID);
    void loadChanges(const QString &sessionID);
    void loadExtras(const QString &sessionID);
    void resync(const QString &sessionID);
    void refreshMcp();
    void refreshLists(std::function<void(bool ok, QString error)> done);
    void stopConnection();
    bool isBusy(const QString &sessionID) const;
    ModelRef activeModelRef() const;
    QJsonObject findModel(const ModelRef &ref) const;
    ThreadModel::Input threadInput(const QString &sessionID) const;
    void updateUsage();
    bool current(const OpencodeApi *api) const { return api && api == m_api; }

    Bob *m_bob;
    Prefs *m_prefs;
    QPointer<OpencodeApi> m_api;
    QPointer<EventStream> m_events;
    QJsonObject m_connection;
    QString m_phase = QStringLiteral("setup");
    QString m_connectError;
    QString m_serverVersion;
    QString m_stream = QStringLiteral("closed");
    QString m_streamDetail;
    bool m_awaitingJoin = false;
    // Said once connected: bob already ran with another bob.toml
    QString m_joinNotice;

    SessionData m_data;
    QHash<QString, bool> m_loading;
    QString m_currentId;
    QJsonArray m_providers;
    ModelRef m_defaultModel;
    QJsonArray m_agents;
    QString m_defaultAgent = QStringLiteral("build");
    QJsonObject m_mcpStatus;
    QVariantMap m_notice;
    QVariantMap m_usage;
    QHash<QString, bool> m_expanded;
    QStringList m_sent;
    ChatLogic::Recall m_recall;

    SessionListModel *m_sessionList;
    ThreadModel *m_thread;
    QHash<QString, ThreadModel *> m_children;
    QSet<QString> m_dirtySessions;
    int m_dirty = 0;
    QTimer m_flush;
    QTimer m_mcpTimer;
    QTimer m_usageTimer;
    QTimer m_clock;
    QHash<QString, QTimer *> m_resync;
};
