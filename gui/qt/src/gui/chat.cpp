#include "chat.h"

#include "bob.h"
#include "connection.h"
#include "desktop.h"
#include "prefs.h"
#include "sessionlistmodel.h"
#include "turns.h"

#include <QDateTime>
#include <QGuiApplication>
#include <QClipboard>

#include <memory>

namespace {

Chat *instance = nullptr;

QString str(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toString();
}

QJsonObject obj(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toObject();
}

double now()
{
    return double(QDateTime::currentMSecsSinceEpoch());
}

struct McpState
{
    const char *tone;
    const char *text;
};

McpState mcpState(const QString &status)
{
    if (status == QStringLiteral("connected"))
        return {"ok", "connected"};
    if (status == QStringLiteral("connecting"))
        return {"warn pulse", "connecting…"};
    if (status == QStringLiteral("disabled"))
        return {"", "switched off in OpenCode's configuration"};
    if (status == QStringLiteral("failed"))
        return {"error", "cannot connect"};
    if (status == QStringLiteral("needs_auth"))
        return {"warn", "needs authorization"};
    if (status == QStringLiteral("needs_client_registration"))
        return {"warn", "needs client registration"};
    return {"warn", nullptr};
}

}  // namespace

Chat::Chat(Bob *bob, Prefs *prefs, QObject *parent)
    : QObject(parent)
    , m_bob(bob)
    , m_prefs(prefs)
    , m_client(new ChatClient(this))
    , m_sessionList(new SessionListModel(this))
    , m_thread(new ThreadModel(false, this))
{
    m_flush.setSingleShot(true);
    m_flush.setInterval(16);
    connect(&m_flush, &QTimer::timeout, this, &Chat::flush);
    // the totals of a running chat include the time it has been answering
    m_usageTimer.setInterval(500);
    connect(&m_usageTimer, &QTimer::timeout, this, [this] { touch(Usage); });
    // relative times and day groups in the chat list
    m_clock.setInterval(30000);
    m_clock.start();
    connect(&m_clock, &QTimer::timeout, this, [this] { touch(SessionList); });
    connect(m_bob, &Bob::statusChanged, this, &Chat::onBobStatus);
    connect(m_prefs, &Prefs::changed, this, &Chat::modelsChanged);
    connect(m_client, &ChatClient::opened, this, &Chat::onOpened);
    connect(m_client, &ChatClient::openFailed, this, [this](const QString &error) {
        setPhase(QStringLiteral("setup"), error);
    });
    connect(m_client, &ChatClient::unauthorized, this, [this] {
        disconnect();
        setPhase(QStringLiteral("setup"),
                 QStringLiteral("The server no longer accepts this password. Paste the connection bob printed "
                                "for its current launch."));
    });
    connect(m_client, &ChatClient::changed, this, &Chat::onChanged);
    connect(m_client, &ChatClient::sessionDeleted, this, &Chat::onSessionDeleted);
    connect(m_client, &ChatClient::mcpChanged, this, &Chat::mcpChanged);
    connect(m_client, &ChatClient::streamChanged, this, &Chat::streamChanged);
    connect(m_client, &ChatClient::failed, this, &Chat::fail);
    m_thread->update(threadInput(QString()));

    // Connect to the bob that Bob holds or is joining: it joins by itself when the app opens
    // (unless that is switched off).
    const BobStatus status = m_bob->bobStatus();
    if (status.state == BobStatus::Joined || status.state == BobStatus::Joining)
        joinBob();
    else if (status.state == BobStatus::Lost)
        m_connectError = status.reason;
    else
        m_connectError = status.error;
}

Chat::~Chat() = default;

Chat *Chat::create(QQmlEngine *, QJSEngine *)
{
    QJSEngine::setObjectOwnership(instance, QJSEngine::CppOwnership);
    return instance;
}

void Chat::setInstance(Chat *chat)
{
    instance = chat;
}

void Chat::setPhase(const QString &phase, const QString &error)
{
    m_phase = phase;
    m_connectError = error;
    emit phaseChanged();
}

void Chat::fail(const ApiError &error)
{
    m_notice = {{QStringLiteral("text"), error.message}, {QStringLiteral("tone"), QStringLiteral("error")}};
    emit noticeChanged();
}

ChatClient::Done Chat::failNotice()
{
    return [this](const std::optional<ApiError> &error) {
        if (error)
            fail(*error);
    };
}

void Chat::touch(int dirty)
{
    m_dirty |= dirty;
    if (!m_flush.isActive())
        m_flush.start();
}

// ---------- what the window reads ----------

QString Chat::title() const
{
    if (m_currentId.isEmpty())
        return QStringLiteral("New chat");
    const QString title = str(m_client->data().sessions.value(m_currentId), "title");
    return title.isEmpty() ? QStringLiteral("Untitled") : title;
}

bool Chat::hasMessages() const
{
    return !m_currentId.isEmpty() && !m_client->data().messages.value(m_currentId).isEmpty();
}

bool Chat::compacting() const
{
    const QJsonValue compacting = obj(m_client->data().sessions.value(m_currentId), "time").value(QStringLiteral("compacting"));
    return !compacting.isUndefined() && !compacting.isNull() && compacting.toVariant().toBool();
}

QString Chat::revertID() const
{
    return str(obj(m_client->data().sessions.value(m_currentId), "revert"), "messageID");
}

bool Chat::isBusy(const QString &sessionID) const
{
    if (sessionID.isEmpty())
        return false;
    const QString type = str(m_client->data().status.value(sessionID), "type");
    return !type.isEmpty() && type != QStringLiteral("idle");
}

bool Chat::running() const
{
    if (busy())
        return true;
    for (auto it = m_children.cbegin(); it != m_children.cend(); ++it) {
        if (isBusy(it.key()))
            return true;
    }
    return false;
}

ModelRef Chat::activeModelRef() const
{
    const ModelRef picked = ModelRef::fromKey(m_prefs->model());
    if (picked.isValid() && !m_client->model(picked).isEmpty())
        return picked;
    return m_client->defaultModel();
}

QVariantList Chat::modelOptions() const
{
    QVariantList options;
    const QJsonArray providers = m_client->providers();
    for (const QJsonValue &value : providers) {
        const QJsonObject provider = value.toObject();
        const QJsonObject models = obj(provider, "models");
        for (auto it = models.begin(); it != models.end(); ++it) {
            const QJsonObject model = it.value().toObject();
            const QString name = str(model, "name").isEmpty() ? it.key() : str(model, "name");
            options << QVariantMap{
                {QStringLiteral("key"), str(provider, "id") + QLatin1Char('/') + it.key()},
                {QStringLiteral("label"), providers.size() > 1 ? str(provider, "name") + QStringLiteral(" · ") + name : name},
            };
        }
    }
    return options;
}

QString Chat::activeModelLabel() const
{
    const ModelRef ref = activeModelRef();
    for (const QVariant &option : modelOptions()) {
        if (option.toMap().value(QStringLiteral("key")).toString() == ref.key())
            return option.toMap().value(QStringLiteral("label")).toString();
    }
    return ref.isValid() ? ref.modelID : QStringLiteral("no model");
}

QString Chat::thinkingVariant() const
{
    return ChatLogic::noThinkingVariant(m_client->model(activeModelRef()));
}

QStringList Chat::agentOptions() const
{
    QStringList names;
    for (const QJsonValue &value : m_client->agents()) {
        const QJsonObject agent = value.toObject();
        if (str(agent, "mode") != QStringLiteral("subagent") && !agent.value(QStringLiteral("hidden")).toBool())
            names << str(agent, "name");
    }
    return names;
}

QString Chat::activeAgent() const
{
    return m_prefs->agent().isEmpty() ? m_client->defaultAgent() : m_prefs->agent();
}

QVariantList Chat::mcpServers() const
{
    QVariantList servers;
    const QJsonObject mcp = m_client->mcpStatus();
    for (auto it = mcp.begin(); it != mcp.end(); ++it) {
        const QJsonObject entry = it.value().toObject();
        const QString status = str(entry, "status");
        const McpState state = mcpState(status);
        const QString text = state.text ? QString::fromUtf8(state.text) : status;
        const QString error = str(entry, "error");
        const bool retry = status != QStringLiteral("connected") && status != QStringLiteral("connecting")
            && status != QStringLiteral("disabled");
        const QString detail = QStringLiteral("MCP server %1: %2%3")
                                   .arg(it.key(), text, error.isEmpty() ? QString() : QStringLiteral(" (%1)").arg(error));
        servers << QVariantMap{
            {QStringLiteral("name"), it.key()},
            {QStringLiteral("status"), status},
            {QStringLiteral("error"), error},
            {QStringLiteral("tone"), QString::fromUtf8(state.tone)},
            {QStringLiteral("text"), text},
            {QStringLiteral("retry"), retry},
            {QStringLiteral("detail"), retry ? detail + QStringLiteral(". Click to connect again.") : detail},
        };
    }
    return servers;
}

QVariantList Chat::permissions() const
{
    QList<QJsonObject> mine;
    for (const QJsonObject &request : m_client->data().permissions) {
        if (str(request, "sessionID") == m_currentId)
            mine << request;
    }
    std::sort(mine.begin(), mine.end(), [](const QJsonObject &a, const QJsonObject &b) { return str(a, "id") < str(b, "id"); });
    const QList<MessageEntry> entries = m_client->data().messages.value(m_currentId);
    QVariantList list;
    for (const QJsonObject &request : mine) {
        const QString callID = str(obj(request, "tool"), "callID");
        QJsonObject tool;
        for (auto entry = entries.crbegin(); entry != entries.crend() && tool.isEmpty() && !callID.isEmpty(); ++entry) {
            for (const QJsonObject &part : entry->parts) {
                if (str(part, "type") == QStringLiteral("tool") && str(part, "callID") == callID) {
                    tool = part;
                    break;
                }
            }
        }
        const ChatLogic::PermissionView view = ChatLogic::describePermission(request, tool);
        QStringList always;
        for (const QJsonValue &pattern : request.value(QStringLiteral("always")).toArray())
            always << pattern.toString();
        list << QVariantMap{
            {QStringLiteral("id"), str(request, "id")},
            {QStringLiteral("title"), view.title},
            {QStringLiteral("detail"), view.detail},
            {QStringLiteral("tool"), str(tool, "tool")},
            {QStringLiteral("permission"), str(request, "permission")},
            {QStringLiteral("always"), always},
        };
    }
    return list;
}

QVariantList Chat::questions() const
{
    QList<QJsonObject> mine;
    for (const QJsonObject &request : m_client->data().questions) {
        if (str(request, "sessionID") == m_currentId)
            mine << request;
    }
    std::sort(mine.begin(), mine.end(), [](const QJsonObject &a, const QJsonObject &b) { return str(a, "id") < str(b, "id"); });
    QVariantList list;
    for (const QJsonObject &request : mine)
        list << request.toVariantMap();
    return list;
}

QVariantList Chat::todos() const
{
    return m_client->data().todos.value(m_currentId).toVariantList();
}

QVariantList Chat::changes() const
{
    const QList<PromptChanges> changes = m_client->data().changes.value(m_currentId);
    const QString revert = revertID();
    QVariantList list;
    for (auto it = changes.crbegin(); it != changes.crend(); ++it) {
        list << QVariantMap{
            {QStringLiteral("messageID"), it->messageID},
            {QStringLiteral("prompt"), it->prompt},
            {QStringLiteral("undone"), !revert.isEmpty() && it->messageID >= revert},
            {QStringLiteral("files"), it->files.toVariantList()},
        };
    }
    return list;
}

int Chat::changedFiles() const
{
    QSet<QString> files;
    for (const PromptChanges &change : m_client->data().changes.value(m_currentId)) {
        for (const QJsonValue &file : change.files)
            files.insert(str(file.toObject(), "file"));
    }
    return int(files.size());
}

QString Chat::lostReason() const
{
    const BobStatus status = m_bob->bobStatus();
    if (status.state != BobStatus::Lost || status.container != str(m_client->connection(), "container"))
        return {};
    return status.reason;
}

// ---------- refreshing the views ----------

ThreadModel::Input Chat::threadInput(const QString &sessionID) const
{
    ThreadModel::Input input;
    auto entries = m_client->data().messages.constFind(sessionID);
    input.entries = sessionID.isEmpty() || entries == m_client->data().messages.cend() ? nullptr : &entries.value();
    input.loading = m_client->isLoading(sessionID);
    input.busy = isBusy(sessionID);
    for (const QJsonObject &request : m_client->data().permissions) {
        if (str(request, "sessionID") == sessionID)
            input.permissions << request;
    }
    input.revertID = str(obj(m_client->data().sessions.value(sessionID), "revert"), "messageID");
    input.sessionError = m_client->data().errors.value(sessionID);
    input.status = m_client->data().status.value(sessionID);
    input.expanded = &m_expanded;
    return input;
}

void Chat::flush()
{
    const int dirty = m_dirty;
    m_dirty = 0;
    if (dirty & SessionList) {
        QSet<QString> waiting;
        for (const QJsonObject &request : m_client->data().permissions)
            waiting.insert(str(request, "sessionID"));
        for (const QJsonObject &request : m_client->data().questions)
            waiting.insert(str(request, "sessionID"));
        const QDateTime clock = QDateTime::currentDateTime();
        QList<SessionListModel::Row> rows;
        for (const QJsonObject &session : sortedSessions(m_client->data().sessions)) {
            const QString id = str(session, "id");
            const double updated = obj(session, "time").value(QStringLiteral("updated")).toDouble();
            const QString type = str(m_client->data().status.value(id), "type");
            rows << SessionListModel::Row{id,
                                          str(session, "title"),
                                          updated,
                                          ChatLogic::dayGroup(updated, clock),
                                          type == QStringLiteral("busy") || type == QStringLiteral("retry"),
                                          waiting.contains(id)};
        }
        m_sessionList->update(rows);
    }
    if (dirty & Thread)
        m_thread->update(threadInput(m_currentId));
    if (dirty & Children) {
        for (auto it = m_children.cbegin(); it != m_children.cend(); ++it)
            it.value()->update(threadInput(it.key()));
    }
    if (dirty & Usage)
        updateUsage();
    if (dirty & Current) {
        if (running() != m_usageTimer.isActive()) {
            if (running())
                m_usageTimer.start();
            else
                m_usageTimer.stop();
        }
        emit currentChanged();
    }
    if (dirty & Requests)
        emit requestsChanged();
    if (dirty & Todos)
        emit todosChanged();
    if (dirty & Changes)
        emit changesChanged();
}

void Chat::updateUsage()
{
    const QList<MessageEntry> entries = m_client->data().messages.value(m_currentId);
    const QList<Turn> turns = groupTurns(entries);
    if (m_currentId.isEmpty() || turns.isEmpty()) {
        if (!m_usage.isEmpty()) {
            m_usage.clear();
            emit usageChanged();
        }
        return;
    }
    Tokens tokens;
    double cost = 0;
    double time = 0;
    int tools = 0;
    std::optional<TurnStats> last;
    const double clock = now();
    for (const Turn &turn : turns) {
        const TurnStats stats = turnStats(entries, turn, clock);
        tokens += stats.tokens;
        cost += stats.cost;
        time += stats.elapsedMs;
        tools += stats.toolCalls;
        // a stopped or running turn has no token count yet: keep the last one that has
        if (stats.contextTokens > 0)
            last = stats;
    }
    const double context = last ? last->contextTokens : 0;
    const double limit = last ? obj(m_client->model({last->providerID, last->modelID}), "limit").value(QStringLiteral("context")).toDouble() : 0;
    const QVariantMap usage{
        {QStringLiteral("turns"), int(turns.size())},
        {QStringLiteral("time"), formatDuration(time)},
        {QStringLiteral("input"), formatTokens(tokens.input)},
        {QStringLiteral("output"), formatTokens(tokens.output)},
        {QStringLiteral("reasoning"), tokens.reasoning > 0 ? formatTokens(tokens.reasoning) : QString()},
        {QStringLiteral("cached"), tokens.cacheRead > 0 ? formatTokens(tokens.cacheRead) : QString()},
        {QStringLiteral("context"), formatTokens(context)},
        {QStringLiteral("contextTokens"), context},
        {QStringLiteral("limit"), limit > 0 ? formatTokens(limit) : QString()},
        {QStringLiteral("limitTokens"), limit},
        {QStringLiteral("fill"), limit > 0 ? qMin(1.0, context / limit) : 0.0},
        {QStringLiteral("tools"), tools},
        {QStringLiteral("cost"), cost > 0 ? formatCost(cost) : QString()},
    };
    if (usage != m_usage) {
        m_usage = usage;
        emit usageChanged();
    }
}

// ---------- bob ----------

void Chat::onBobStatus()
{
    emit lostChanged();
    evaluateJoin();
}

void Chat::joinBob()
{
    dismissRestart();
    setPhase(QStringLiteral("starting"));
    m_awaitingJoin = true;
    m_bob->join();
    evaluateJoin();
}

void Chat::evaluateJoin()
{
    if (!m_awaitingJoin)
        return;
    const BobStatus status = m_bob->bobStatus();
    if (status.state == BobStatus::Joining)
        return;
    m_awaitingJoin = false;
    if (status.state != BobStatus::Joined) {
        setPhase(QStringLiteral("setup"), status.state == BobStatus::Idle ? status.error : QString());
        return;
    }
    const auto connection = Connection::fromJson(status.connection);
    if (!connection) {
        setPhase(QStringLiteral("setup"), QStringLiteral("bob's broker sent a connection without URL or password"));
        return;
    }
    m_offerRestart = status.runningConfig != status.config;
    connectTo(connection->toVariantMap());
}

void Chat::leaveBob()
{
    m_awaitingJoin = false;
    m_bob->leave();
    disconnect();
}

void Chat::restartBob()
{
    disconnect();
    setPhase(QStringLiteral("starting"));
    m_bob->restart();
    m_awaitingJoin = true;
    evaluateJoin();
}

void Chat::dismissRestart()
{
    if (!m_restartOffer)
        return;
    m_restartOffer = false;
    emit restartOfferChanged();
}

// ---------- the connection ----------

void Chat::resetViews()
{
    m_usageTimer.stop();
    m_currentId.clear();
    qDeleteAll(m_children);
    m_children.clear();
    m_thread->reset();
}

void Chat::disconnect()
{
    m_client->close();
    dismissRestart();
    resetViews();
    setPhase(QStringLiteral("setup"));
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
}

void Chat::connectTo(const QVariantMap &connection)
{
    resetViews();
    m_client->open(QJsonObject::fromVariantMap(connection));
    setPhase(QStringLiteral("connecting"));
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
}

void Chat::onOpened()
{
    const QList<QJsonObject> sessions = sortedSessions(m_client->data().sessions);
    m_currentId = sessions.isEmpty() ? QString() : str(sessions.first(), "id");
    m_client->setFocus(m_currentId);
    setPhase(QStringLiteral("ready"));
    emit modelsChanged();
    emit mcpChanged();
    if (!m_currentId.isEmpty()) {
        m_client->loadMessages(m_currentId);
        m_client->loadExtras(m_currentId);
    }
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
    if (m_offerRestart) {
        m_offerRestart = false;
        m_restartOffer = true;
        emit restartOfferChanged();
    }
}

// Which views show what changed: the open chat's, a sub-agent's steps, or the chat list.
void Chat::onChanged(const QString &sessionID, int areas)
{
    const bool all = sessionID.isEmpty();
    const bool open = all || sessionID == m_currentId;
    const bool child = all || m_children.contains(sessionID);
    int dirty = 0;
    if (areas & EventEffect::Sessions)
        dirty |= SessionList | (open ? Thread | Current : 0);
    if (areas & EventEffect::Status)
        dirty |= SessionList | (open ? Thread | Current | Usage : 0) | (child ? Children | Current : 0);
    if (areas & EventEffect::Messages)
        dirty |= (open ? Thread | Current | Usage : 0) | (child ? Children : 0);
    if (areas & EventEffect::Requests)
        dirty |= SessionList | Children | (open ? Thread | Requests : 0);
    if (areas & EventEffect::Errors)
        dirty |= open ? Thread : 0;
    if (areas & EventEffect::Todos)
        dirty |= open ? Todos : 0;
    if (areas & EventEffect::Changes)
        dirty |= open ? Changes : 0;
    touch(dirty);
}

void Chat::onSessionDeleted(const QString &sessionID)
{
    if (sessionID != m_currentId)
        return;
    const QList<QJsonObject> sessions = sortedSessions(m_client->data().sessions);
    selectSession(sessions.isEmpty() ? QString() : str(sessions.first(), "id"));
}

// ---------- actions ----------

void Chat::selectSession(const QString &id)
{
    if (id != m_currentId)
        m_thread->reset();
    m_currentId = id;
    m_client->setFocus(id);
    m_recall = {};
    if (!m_notice.isEmpty()) {
        m_notice.clear();
        emit noticeChanged();
    }
    if (!id.isEmpty() && !m_client->data().messages.contains(id))
        m_client->loadMessages(id);
    if (!id.isEmpty())
        m_client->loadExtras(id);
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
}

void Chat::send(const QString &text, bool typed)
{
    const QString prompt = text.trimmed();
    if (prompt.isEmpty() || !m_client->isOpen())
        return;
    if (typed)
        m_sent << prompt;
    m_recall = {};
    auto submit = [this, text](const QString &sessionID) {
        const ModelRef model = activeModelRef();
        const QString variant = m_prefs->thinking() ? QString() : ChatLogic::noThinkingVariant(m_client->model(model));
        dismissNotice();
        m_client->prompt(sessionID, text, model, m_prefs->agent(), variant, failNotice());
    };
    if (!m_currentId.isEmpty()) {
        submit(m_currentId);
        return;
    }
    m_client->createSession([this, submit](const QString &id, const std::optional<ApiError> &error) {
        if (error) {
            fail(*error);
            return;
        }
        m_thread->reset();
        m_currentId = id;
        m_client->setFocus(id);
        touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
        submit(id);
    });
}

void Chat::abort()
{
    if (!m_currentId.isEmpty())
        m_client->abort(m_currentId, failNotice());
}

void Chat::renameSession(const QString &id, const QString &title)
{
    m_client->renameSession(id, title, failNotice());
}

void Chat::deleteSession(const QString &id)
{
    m_client->deleteSession(id, failNotice());
}

void Chat::replyPermission(const QString &id, const QString &reply)
{
    const bool sent = m_client->replyPermission(id, reply, [this, id](const std::optional<ApiError> &error) {
        if (!error)
            return;
        fail(*error);
        emit permissionReplyFailed(id);
    });
    if (!sent)
        emit permissionReplyFailed(id);
}

void Chat::replyQuestion(const QString &id, const QVariantList &answers)
{
    m_client->replyQuestion(id, QJsonArray::fromVariantList(answers), failNotice());
}

void Chat::rejectQuestion(const QString &id)
{
    m_client->rejectQuestion(id, failNotice());
}

void Chat::compact()
{
    const ModelRef model = activeModelRef();
    if (!m_currentId.isEmpty() && model.isValid())
        m_client->compact(m_currentId, model, failNotice());
}

void Chat::revert(const QString &messageID)
{
    const QString sessionID = m_currentId;
    if (sessionID.isEmpty())
        return;
    m_client->revert(sessionID, messageID, [this, sessionID, messageID](const std::optional<ApiError> &error) {
        if (error) {
            fail(*error);
            return;
        }
        // like OpenCode's /undo: the undone prompt goes back into the composer
        for (const MessageEntry &entry : m_client->data().messages.value(sessionID)) {
            if (entry.id() == messageID) {
                const QString text = promptText(entry);
                if (!text.isEmpty())
                    emit composerRequest(text, true);
                break;
            }
        }
    });
}

void Chat::unrevert()
{
    if (m_currentId.isEmpty())
        return;
    m_client->unrevert(m_currentId, [this](const std::optional<ApiError> &error) {
        if (error) {
            fail(*error);
            return;
        }
        // the undone prompt went into the composer; redo takes it back out
        emit composerRequest(QString(), true);
    });
}

void Chat::fork(const QString &messageID)
{
    if (m_currentId.isEmpty())
        return;
    m_client->fork(m_currentId, messageID, [this](const QString &id, const std::optional<ApiError> &error) {
        if (error)
            fail(*error);
        else
            selectSession(id);
    });
}

void Chat::reconnectMcp(const QString &name)
{
    m_client->reconnectMcp(name, failNotice());
}

void Chat::setNotice(const QString &text)
{
    m_notice = {{QStringLiteral("text"), text}, {QStringLiteral("tone"), QStringLiteral("info")}};
    emit noticeChanged();
}

void Chat::dismissNotice()
{
    if (m_notice.isEmpty())
        return;
    m_notice.clear();
    emit noticeChanged();
}

void Chat::setModel(const QString &key)
{
    m_prefs->setModel(key);
}

void Chat::setAgent(const QString &name)
{
    m_prefs->setAgent(name);
}

void Chat::setExpanded(const QString &key, bool expanded)
{
    m_expanded.insert(key, expanded);
    touch(Thread | Children);
}

ThreadModel *Chat::stepsModel(const QString &sessionID)
{
    if (sessionID.isEmpty())
        return nullptr;
    ThreadModel *model = m_children.value(sessionID);
    if (!model) {
        model = new ThreadModel(true, this);
        QJSEngine::setObjectOwnership(model, QJSEngine::CppOwnership);
        m_children.insert(sessionID, model);
        if (!m_client->data().messages.contains(sessionID) && !m_client->isLoading(sessionID))
            m_client->loadMessages(sessionID);
        model->update(threadInput(sessionID));
        touch(Current);
    }
    return model;
}

void Chat::copyChat()
{
    if (m_currentId.isEmpty())
        return;
    QGuiApplication::clipboard()->setText(ChatLogic::chatMarkdown(m_client->data().sessions.value(m_currentId),
                                                                  m_client->data().messages.value(m_currentId), false,
                                                                  QDateTime::currentDateTime(), now()));
    setNotice(QStringLiteral("Copied the chat as Markdown."));
}

void Chat::exportChat()
{
    if (m_currentId.isEmpty())
        return;
    const QJsonObject session = m_client->data().sessions.value(m_currentId);
    const QString markdown = ChatLogic::chatMarkdown(session, m_client->data().messages.value(m_currentId), false,
                                                     QDateTime::currentDateTime(), now());
    Desktop desktop;
    const QVariantMap saved = desktop.saveDownload(ChatLogic::exportFileName(session), markdown);
    if (saved.contains(QStringLiteral("path")))
        setNotice(QStringLiteral("Saved the chat to %1.").arg(saved.value(QStringLiteral("path")).toString()));
    else
        setNotice(QStringLiteral("Could not save the chat: %1").arg(saved.value(QStringLiteral("error")).toString()));
}

QVariantMap Chat::recall(bool up, const QString &currentText)
{
    const QStringList history = ChatLogic::promptHistory(m_client->data().messages.value(m_currentId), m_sent);
    const auto step = ChatLogic::recallStep(history, m_recall, currentText, up);
    if (!step)
        return {};
    m_recall = step->recall;
    return {{QStringLiteral("text"), step->text}};
}

void Chat::endRecall()
{
    m_recall = {};
}
