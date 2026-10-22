#include "chat.h"

#include "bob.h"
#include "desktop.h"
#include "eventstream.h"
#include "prefs.h"
#include "sessionlistmodel.h"
#include "turns.h"

#include <QDateTime>
#include <QGuiApplication>
#include <QClipboard>

#include <memory>

namespace {

Chat *instance = nullptr;
// How often the MCP servers' status is asked again while connected: it changes without an event
// when one drops or comes back.
const int mcpPollMs = 20000;

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

QList<MessageEntry> parseEntries(const QJsonValue &value)
{
    QList<MessageEntry> entries;
    for (const QJsonValue &item : value.toArray()) {
        const QJsonObject object = item.toObject();
        MessageEntry entry;
        entry.info = object.value(QStringLiteral("info")).toObject();
        for (const QJsonValue &part : object.value(QStringLiteral("parts")).toArray())
            entry.parts << part.toObject();
        entries << entry;
    }
    return entries;
}

ModelRef parseModel(const QString &value)
{
    const qsizetype slash = value.indexOf(QLatin1Char('/'));
    if (slash <= 0)
        return {};
    return {value.left(slash), value.mid(slash + 1)};
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

// Waits for several requests, then calls `done` once.
struct Gather
{
    int left = 0;
    std::function<void()> done;
    void finish()
    {
        if (--left == 0 && done)
            done();
    }
};

}  // namespace

Chat::Chat(Bob *bob, Prefs *prefs, QObject *parent)
    : QObject(parent)
    , m_bob(bob)
    , m_prefs(prefs)
    , m_sessionList(new SessionListModel(this))
    , m_thread(new ThreadModel(false, this))
{
    m_flush.setSingleShot(true);
    m_flush.setInterval(16);
    connect(&m_flush, &QTimer::timeout, this, &Chat::flush);
    m_mcpTimer.setInterval(mcpPollMs);
    connect(&m_mcpTimer, &QTimer::timeout, this, &Chat::refreshMcp);
    // the totals of a running chat include the time it has been answering
    m_usageTimer.setInterval(500);
    connect(&m_usageTimer, &QTimer::timeout, this, [this] { touch(Usage); });
    // relative times and day groups in the chat list
    m_clock.setInterval(30000);
    m_clock.start();
    connect(&m_clock, &QTimer::timeout, this, [this] { touch(SessionList); });
    connect(m_bob, &Bob::statusChanged, this, &Chat::onBobStatus);
    connect(m_prefs, &Prefs::changed, this, &Chat::modelsChanged);
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

Chat::~Chat()
{
    stopConnection();
}

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
    const QString title = str(m_data.sessions.value(m_currentId), "title");
    return title.isEmpty() ? QStringLiteral("Untitled") : title;
}

bool Chat::hasMessages() const
{
    return !m_currentId.isEmpty() && !m_data.messages.value(m_currentId).isEmpty();
}

bool Chat::compacting() const
{
    const QJsonValue compacting = obj(m_data.sessions.value(m_currentId), "time").value(QStringLiteral("compacting"));
    return !compacting.isUndefined() && !compacting.isNull() && compacting.toVariant().toBool();
}

QString Chat::revertID() const
{
    return str(obj(m_data.sessions.value(m_currentId), "revert"), "messageID");
}

bool Chat::isBusy(const QString &sessionID) const
{
    if (sessionID.isEmpty())
        return false;
    const QString type = str(m_data.status.value(sessionID), "type");
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

QJsonObject Chat::findModel(const ModelRef &ref) const
{
    if (!ref.isValid())
        return {};
    for (const QJsonValue &provider : m_providers) {
        if (str(provider.toObject(), "id") == ref.providerID)
            return obj(obj(provider.toObject(), "models"), ref.modelID.toUtf8().constData());
    }
    return {};
}

ModelRef Chat::activeModelRef() const
{
    const ModelRef picked = parseModel(m_prefs->model());
    if (picked.isValid() && !findModel(picked).isEmpty())
        return picked;
    return m_defaultModel;
}

QVariantList Chat::modelOptions() const
{
    QVariantList options;
    for (const QJsonValue &value : m_providers) {
        const QJsonObject provider = value.toObject();
        const QJsonObject models = obj(provider, "models");
        for (auto it = models.begin(); it != models.end(); ++it) {
            const QJsonObject model = it.value().toObject();
            const QString name = str(model, "name").isEmpty() ? it.key() : str(model, "name");
            options << QVariantMap{
                {QStringLiteral("key"), str(provider, "id") + QLatin1Char('/') + it.key()},
                {QStringLiteral("label"), m_providers.size() > 1 ? str(provider, "name") + QStringLiteral(" · ") + name : name},
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
    return ChatLogic::noThinkingVariant(findModel(activeModelRef()));
}

QStringList Chat::agentOptions() const
{
    QStringList names;
    for (const QJsonValue &value : m_agents) {
        const QJsonObject agent = value.toObject();
        if (str(agent, "mode") != QStringLiteral("subagent") && !agent.value(QStringLiteral("hidden")).toBool())
            names << str(agent, "name");
    }
    return names;
}

QString Chat::activeAgent() const
{
    return m_prefs->agent().isEmpty() ? m_defaultAgent : m_prefs->agent();
}

QVariantList Chat::mcpServers() const
{
    QVariantList servers;
    for (auto it = m_mcpStatus.begin(); it != m_mcpStatus.end(); ++it) {
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
    for (const QJsonObject &request : m_data.permissions) {
        if (str(request, "sessionID") == m_currentId)
            mine << request;
    }
    std::sort(mine.begin(), mine.end(), [](const QJsonObject &a, const QJsonObject &b) { return str(a, "id") < str(b, "id"); });
    const QList<MessageEntry> entries = m_data.messages.value(m_currentId);
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
    for (const QJsonObject &request : m_data.questions) {
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
    return m_data.todos.value(m_currentId).toVariantList();
}

QVariantList Chat::changes() const
{
    const QList<PromptChanges> changes = m_data.changes.value(m_currentId);
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
    for (const PromptChanges &change : m_data.changes.value(m_currentId)) {
        for (const QJsonValue &file : change.files)
            files.insert(str(file.toObject(), "file"));
    }
    return int(files.size());
}

QString Chat::lostReason() const
{
    const BobStatus status = m_bob->bobStatus();
    if (status.state != BobStatus::Lost || status.container != str(m_connection, "container"))
        return {};
    return status.reason;
}

// ---------- refreshing the views ----------

ThreadModel::Input Chat::threadInput(const QString &sessionID) const
{
    ThreadModel::Input input;
    auto entries = m_data.messages.constFind(sessionID);
    input.entries = sessionID.isEmpty() || entries == m_data.messages.cend() ? nullptr : &entries.value();
    input.loading = m_loading.value(sessionID);
    input.busy = isBusy(sessionID);
    for (const QJsonObject &request : m_data.permissions) {
        if (str(request, "sessionID") == sessionID)
            input.permissions << request;
    }
    input.revertID = str(obj(m_data.sessions.value(sessionID), "revert"), "messageID");
    input.sessionError = m_data.errors.value(sessionID);
    input.status = m_data.status.value(sessionID);
    input.expanded = &m_expanded;
    return input;
}

void Chat::flush()
{
    const int dirty = m_dirty;
    m_dirty = 0;
    if (dirty & SessionList) {
        QSet<QString> waiting;
        for (const QJsonObject &request : m_data.permissions)
            waiting.insert(str(request, "sessionID"));
        for (const QJsonObject &request : m_data.questions)
            waiting.insert(str(request, "sessionID"));
        const QDateTime clock = QDateTime::currentDateTime();
        QList<SessionListModel::Row> rows;
        for (const QJsonObject &session : sortedSessions(m_data.sessions)) {
            const QString id = str(session, "id");
            const double updated = obj(session, "time").value(QStringLiteral("updated")).toDouble();
            const QString type = str(m_data.status.value(id), "type");
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
    const QList<MessageEntry> entries = m_data.messages.value(m_currentId);
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
    const double limit = last ? obj(findModel({last->providerID, last->modelID}), "limit").value(QStringLiteral("context")).toDouble() : 0;
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
    const auto connection = ChatLogic::toConnection(status.connection);
    if (!connection) {
        setPhase(QStringLiteral("setup"), QStringLiteral("bob's broker sent a connection without URL or password"));
        return;
    }
    m_joinNotice = status.runningConfig != status.config
        ? QStringLiteral("bob already runs with %1, started by another program; this app uses it instead of %2.")
              .arg(status.runningConfig, status.config)
        : QString();
    connectTo(connection->toVariantMap());
}

void Chat::leaveBob()
{
    m_awaitingJoin = false;
    m_bob->leave();
    disconnect();
}

// ---------- the connection ----------

void Chat::stopConnection()
{
    if (m_events) {
        m_events->stop();
        m_events->deleteLater();
    }
    m_events = nullptr;
    if (m_api)
        m_api->deleteLater();
    m_api = nullptr;
    m_mcpTimer.stop();
    m_usageTimer.stop();
    qDeleteAll(m_resync);
    m_resync.clear();
}

void Chat::disconnect()
{
    stopConnection();
    m_data = SessionData();
    m_loading.clear();
    m_connection = {};
    m_currentId.clear();
    m_stream = QStringLiteral("closed");
    m_streamDetail.clear();
    qDeleteAll(m_children);
    m_children.clear();
    m_thread->reset();
    setPhase(QStringLiteral("setup"));
    emit streamChanged();
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
}

void Chat::connectTo(const QVariantMap &connectionMap)
{
    stopConnection();
    const QJsonObject connection = QJsonObject::fromVariantMap(connectionMap);
    auto *api = new OpencodeApi(connection, this);
    m_api = api;
    m_connection = connection;
    m_data = SessionData();
    m_loading.clear();
    m_currentId.clear();
    qDeleteAll(m_children);
    m_children.clear();
    m_thread->reset();
    setPhase(QStringLiteral("connecting"));
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);

    auto failed = [this, api](const QString &error) {
        if (!current(api))
            return;
        stopConnection();
        m_connection = {};
        setPhase(QStringLiteral("setup"), error);
    };

    api->health([this, api, failed](const QJsonValue &health, const std::optional<ApiError> &healthError) {
        if (!current(api))
            return;
        // Older servers have no /global/health; the session list proves the password.
        if (healthError && healthError->status != 404) {
            failed(healthError->message);
            return;
        }
        m_serverVersion = str(health.toObject(), "version");

        struct Results
        {
            QJsonValue providers, config, agents, mcp;
            QString error;
        };
        auto results = std::make_shared<Results>();
        auto gather = std::make_shared<Gather>();
        gather->left = 5;
        gather->done = [this, api, results, failed] {
            if (!current(api))
                return;
            if (!results->error.isEmpty()) {
                failed(results->error);
                return;
            }
            const QJsonObject providers = results->providers.toObject();
            const QJsonObject config = results->config.toObject();
            m_providers = providers.value(QStringLiteral("providers")).toArray();
            m_defaultModel = parseModel(str(config, "model"));
            const QJsonObject defaults = obj(providers, "default");
            if (!m_defaultModel.isValid() && !defaults.isEmpty())
                m_defaultModel = {defaults.begin().key(), defaults.begin().value().toString()};
            m_agents = results->agents.toArray();
            m_defaultAgent = str(config, "default_agent").isEmpty() ? QStringLiteral("build") : str(config, "default_agent");
            m_mcpStatus = results->mcp.toObject();
            const QList<QJsonObject> sessions = sortedSessions(m_data.sessions);
            m_currentId = sessions.isEmpty() ? QString() : str(sessions.first(), "id");
            setPhase(QStringLiteral("ready"));
            emit modelsChanged();
            emit mcpChanged();
            if (!m_currentId.isEmpty()) {
                loadMessages(m_currentId);
                loadExtras(m_currentId);
            }
            touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
            if (!m_joinNotice.isEmpty()) {
                m_notice = {{QStringLiteral("text"), m_joinNotice}, {QStringLiteral("tone"), QStringLiteral("info")}};
                m_joinNotice.clear();
                emit noticeChanged();
            }
            m_mcpTimer.start();
            auto *events = new EventStream(api, this);
            m_events = events;
            connect(events, &EventStream::event, this, [this, api](const QJsonObject &event) {
                if (current(api))
                    onEvent(event);
            });
            connect(events, &EventStream::opened, this, [this, api](bool reconnected) {
                if (!current(api) || !reconnected)
                    return;
                refreshLists([this](bool ok, const QString &error) {
                    if (!ok)
                        fail({0, error});
                });
                for (const QString &id : m_data.messages.keys())
                    loadMessages(id);
                refreshMcp();
            });
            connect(events, &EventStream::stateChanged, this, [this, api](const QString &state, const QString &detail) {
                if (!current(api))
                    return;
                m_stream = state;
                m_streamDetail = detail;
                emit streamChanged();
            });
            connect(events, &EventStream::unauthorized, this, [this, api] {
                if (!current(api))
                    return;
                disconnect();
                setPhase(QStringLiteral("setup"),
                         QStringLiteral("The server no longer accepts this password. Paste the connection bob printed "
                                        "for its current launch."));
            });
            events->start();
        };
        api->providers([results, gather](const QJsonValue &value, const std::optional<ApiError> &error) {
            if (error)
                results->error = error->message;
            results->providers = value;
            gather->finish();
        });
        api->config([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
            results->config = value;
            gather->finish();
        });
        api->agents([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
            results->agents = value;
            gather->finish();
        });
        api->mcpStatus([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
            results->mcp = value;
            gather->finish();
        });
        refreshLists([results, gather](bool ok, const QString &error) {
            if (!ok && results->error.isEmpty())
                results->error = error;
            gather->finish();
        });
    });
}

void Chat::refreshLists(std::function<void(bool, QString)> done)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    struct Results
    {
        QJsonValue sessions, status, permissions, questions;
        QString error;
    };
    auto results = std::make_shared<Results>();
    auto gather = std::make_shared<Gather>();
    gather->left = 4;
    gather->done = [this, api, results, done] {
        if (!current(api))
            return;
        if (!results->error.isEmpty()) {
            done(false, results->error);
            return;
        }
        m_data.sessions.clear();
        for (const QJsonValue &session : results->sessions.toArray())
            m_data.sessions.insert(str(session.toObject(), "id"), session.toObject());
        m_data.status.clear();
        const QJsonObject status = results->status.toObject();
        for (auto it = status.begin(); it != status.end(); ++it)
            m_data.status.insert(it.key(), it.value().toObject());
        m_data.permissions.clear();
        for (const QJsonValue &request : results->permissions.toArray())
            m_data.permissions.insert(str(request.toObject(), "id"), request.toObject());
        m_data.questions.clear();
        for (const QJsonValue &request : results->questions.toArray())
            m_data.questions.insert(str(request.toObject(), "id"), request.toObject());
        touch(SessionList | Thread | Children | Current | Requests);
        done(true, {});
    };
    api->sessions([results, gather](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (error)
            results->error = error->message;
        results->sessions = value;
        gather->finish();
    });
    api->sessionStatus([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
        results->status = value;
        gather->finish();
    });
    api->permissions([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
        results->permissions = value;
        gather->finish();
    });
    api->questions([results, gather](const QJsonValue &value, const std::optional<ApiError> &) {
        results->questions = value;
        gather->finish();
    });
}

void Chat::refreshMcp()
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    api->mcpStatus([this, api](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api) || error)
            return;
        if (value.toObject() != m_mcpStatus) {
            m_mcpStatus = value.toObject();
            emit mcpChanged();
        }
    });
}

void Chat::onEvent(const QJsonObject &event)
{
    const QString type = str(event, "type");
    if (type == QStringLiteral("mcp.tools.changed"))
        refreshMcp();
    const EventEffect effect = applyEvent(m_data, event);
    if (effect.changed()) {
        const bool open = !effect.sessionID.isEmpty() && effect.sessionID == m_currentId;
        const bool child = m_children.contains(effect.sessionID);
        int dirty = 0;
        if (effect.areas & EventEffect::Sessions)
            dirty |= SessionList | (open ? Thread | Current : 0);
        if (effect.areas & EventEffect::Status)
            dirty |= SessionList | (open ? Thread | Current | Usage : 0) | (child ? Children | Current : 0);
        if (effect.areas & EventEffect::Messages)
            dirty |= (open ? Thread | Current | Usage : 0) | (child ? Children : 0);
        if (effect.areas & EventEffect::Requests)
            dirty |= SessionList | Children | (open ? Thread | Requests : 0);
        if (effect.areas & EventEffect::Errors)
            dirty |= open ? Thread : 0;
        if (effect.areas & EventEffect::Todos)
            dirty |= open ? Todos : 0;
        if (effect.areas & EventEffect::Changes)
            dirty |= open ? Changes : 0;
        touch(dirty);
    }
    const QJsonObject properties = event.value(QStringLiteral("properties")).toObject();
    if (type == QStringLiteral("session.idle"))
        resync(str(properties, "sessionID"));
    if (type == QStringLiteral("session.deleted") && str(obj(properties, "info"), "id") == m_currentId) {
        const QList<QJsonObject> sessions = sortedSessions(m_data.sessions);
        selectSession(sessions.isEmpty() ? QString() : str(sessions.first(), "id"));
    }
}

void Chat::loadMessages(const QString &sessionID)
{
    OpencodeApi *api = m_api;
    if (!api || sessionID.isEmpty())
        return;
    m_loading.insert(sessionID, true);
    touch(Thread | Children);
    api->messages(sessionID, [this, api, sessionID](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        m_loading.insert(sessionID, false);
        if (error) {
            fail(*error);
        } else {
            m_data.messages.insert(sessionID, parseEntries(value));
            if (sessionID == m_currentId)
                loadChanges(sessionID);
        }
        // a pending request shows its tool call, which these messages may hold
        touch(Thread | Children | Current | Usage | Requests);
    });
}

// The session-wide diff stays empty in OpenCode 1.18, so changes are asked per prompt, and only for
// the prompts whose answers report changed files (a patch part).
void Chat::loadChanges(const QString &sessionID)
{
    OpencodeApi *api = m_api;
    if (!api || !m_data.messages.contains(sessionID))
        return;
    const QList<MessageEntry> entries = m_data.messages.value(sessionID);
    QList<PromptChanges> prompts;
    for (const Turn &turn : groupTurns(entries)) {
        if (turn.user < 0)
            continue;
        bool patched = false;
        for (int index : turn.steps) {
            for (const QJsonObject &part : entries[index].parts)
                patched = patched || str(part, "type") == QStringLiteral("patch");
        }
        if (patched)
            prompts << PromptChanges{turn.id, promptText(entries[turn.user]), {}};
    }
    auto collected = std::make_shared<QList<PromptChanges>>(prompts);
    auto gather = std::make_shared<Gather>();
    gather->left = int(prompts.size()) + 1;
    gather->done = [this, api, sessionID, collected] {
        if (!current(api))
            return;
        QList<PromptChanges> changes;
        for (const PromptChanges &change : *collected) {
            if (!change.files.isEmpty())
                changes << change;
        }
        m_data.changes.insert(sessionID, changes);
        if (sessionID == m_currentId)
            touch(Changes);
    };
    for (int i = 0; i < prompts.size(); ++i) {
        api->diff(sessionID, prompts[i].messageID, [collected, gather, i](const QJsonValue &value, const std::optional<ApiError> &) {
            (*collected)[i].files = value.toArray();
            gather->finish();
        });
    }
    gather->finish();
}

void Chat::loadExtras(const QString &sessionID)
{
    OpencodeApi *api = m_api;
    if (!api || sessionID.isEmpty())
        return;
    api->todos(sessionID, [this, api, sessionID](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (!error) {
            m_data.todos.insert(sessionID, value.toArray());
            if (sessionID == m_currentId)
                touch(Todos);
        }
        loadChanges(sessionID);
    });
}

// After a turn ends, fetch the session once more so a missed event cannot leave it stale.
void Chat::resync(const QString &sessionID)
{
    QTimer *timer = m_resync.value(sessionID);
    if (!timer) {
        timer = new QTimer(this);
        timer->setSingleShot(true);
        timer->setInterval(300);
        connect(timer, &QTimer::timeout, this, [this, sessionID] {
            if (m_data.messages.contains(sessionID))
                loadMessages(sessionID);
            if (sessionID == m_currentId)
                loadExtras(sessionID);
        });
        m_resync.insert(sessionID, timer);
    }
    timer->start();
}

// ---------- actions ----------

void Chat::selectSession(const QString &id)
{
    if (id != m_currentId)
        m_thread->reset();
    m_currentId = id;
    m_recall = {};
    if (!m_notice.isEmpty()) {
        m_notice.clear();
        emit noticeChanged();
    }
    if (!id.isEmpty() && !m_data.messages.contains(id))
        loadMessages(id);
    if (!id.isEmpty())
        loadExtras(id);
    touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
}

void Chat::send(const QString &text)
{
    OpencodeApi *api = m_api;
    const QString prompt = text.trimmed();
    if (!api || prompt.isEmpty())
        return;
    m_sent << prompt;
    m_recall = {};
    auto submit = [this, api, text](const QString &sessionID) {
        const ModelRef model = activeModelRef();
        const QString variant = m_prefs->thinking() ? QString() : ChatLogic::noThinkingVariant(findModel(model));
        m_data.errors.remove(sessionID);
        if (!m_notice.isEmpty()) {
            m_notice.clear();
            emit noticeChanged();
        }
        touch(Thread);
        api->prompt(sessionID, text, model, m_prefs->agent(), variant,
                    [this, api](const QJsonValue &, const std::optional<ApiError> &error) {
                        if (current(api) && error)
                            fail(*error);
                    });
    };
    if (!m_currentId.isEmpty()) {
        submit(m_currentId);
        return;
    }
    api->createSession([this, api, submit](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        const QJsonObject session = value.toObject();
        const QString id = str(session, "id");
        m_data.sessions.insert(id, session);
        m_data.messages.insert(id, {});
        m_thread->reset();
        m_currentId = id;
        touch(SessionList | Thread | Current | Usage | Requests | Todos | Changes);
        submit(id);
    });
}

void Chat::abort()
{
    if (!m_api || m_currentId.isEmpty())
        return;
    OpencodeApi *api = m_api;
    api->abort(m_currentId, [this, api](const QJsonValue &, const std::optional<ApiError> &error) {
        if (current(api) && error)
            fail(*error);
    });
}

void Chat::renameSession(const QString &id, const QString &title)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    api->renameSession(id, title, [this, api, id](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        m_data.sessions.insert(id, value.toObject());
        touch(SessionList | Current);
    });
}

void Chat::deleteSession(const QString &id)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    api->deleteSession(id, [this, api, id](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        m_data.sessions.remove(id);
        m_data.messages.remove(id);
        touch(SessionList);
        if (m_currentId == id) {
            const QList<QJsonObject> sessions = sortedSessions(m_data.sessions);
            selectSession(sessions.isEmpty() ? QString() : str(sessions.first(), "id"));
        }
    });
}

void Chat::replyPermission(const QString &id, const QString &reply)
{
    OpencodeApi *api = m_api;
    if (!api) {
        emit permissionReplyFailed(id);
        return;
    }
    api->replyPermission(id, reply, [this, api, id](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            emit permissionReplyFailed(id);
            return;
        }
        m_data.permissions.remove(id);
        touch(SessionList | Thread | Children | Requests);
    });
}

void Chat::replyQuestion(const QString &id, const QVariantList &answers)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    api->replyQuestion(id, QJsonArray::fromVariantList(answers),
                       [this, api, id](const QJsonValue &, const std::optional<ApiError> &error) {
                           if (!current(api))
                               return;
                           if (error) {
                               fail(*error);
                               return;
                           }
                           m_data.questions.remove(id);
                           touch(SessionList | Requests);
                       });
}

void Chat::rejectQuestion(const QString &id)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    api->rejectQuestion(id, [this, api, id](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        m_data.questions.remove(id);
        touch(SessionList | Requests);
    });
}

void Chat::compact()
{
    OpencodeApi *api = m_api;
    const ModelRef model = activeModelRef();
    if (!api || m_currentId.isEmpty() || !model.isValid())
        return;
    api->compact(m_currentId, model, [this, api](const QJsonValue &, const std::optional<ApiError> &error) {
        if (current(api) && error)
            fail(*error);
    });
}

void Chat::revert(const QString &messageID)
{
    OpencodeApi *api = m_api;
    const QString sessionID = m_currentId;
    if (!api || sessionID.isEmpty())
        return;
    api->revert(sessionID, messageID, [this, api, sessionID, messageID](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        m_data.sessions.insert(sessionID, value.toObject());
        loadExtras(sessionID);
        touch(SessionList | Thread | Current | Changes);
        // like OpenCode's /undo: the undone prompt goes back into the composer
        for (const MessageEntry &entry : m_data.messages.value(sessionID)) {
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
    OpencodeApi *api = m_api;
    const QString sessionID = m_currentId;
    if (!api || sessionID.isEmpty())
        return;
    api->unrevert(sessionID, [this, api, sessionID](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        m_data.sessions.insert(sessionID, value.toObject());
        // the undone prompt went into the composer; redo takes it back out
        emit composerRequest(QString(), true);
        loadExtras(sessionID);
        touch(SessionList | Thread | Current | Changes);
    });
}

void Chat::fork(const QString &messageID)
{
    OpencodeApi *api = m_api;
    if (!api || m_currentId.isEmpty())
        return;
    api->fork(m_currentId, messageID, [this, api](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error) {
            fail(*error);
            return;
        }
        const QJsonObject session = value.toObject();
        m_data.sessions.insert(str(session, "id"), session);
        selectSession(str(session, "id"));
    });
}

void Chat::reconnectMcp(const QString &name)
{
    OpencodeApi *api = m_api;
    if (!api)
        return;
    QJsonObject entry = m_mcpStatus.value(name).toObject();
    entry.insert(QStringLiteral("status"), QStringLiteral("connecting"));
    entry.remove(QStringLiteral("error"));
    m_mcpStatus.insert(name, entry);
    emit mcpChanged();
    api->mcpConnect(name, [this, api](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!current(api))
            return;
        if (error)
            fail(*error);
        refreshMcp();
    });
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
        if (!m_data.messages.contains(sessionID) && !m_loading.value(sessionID))
            loadMessages(sessionID);
        model->update(threadInput(sessionID));
        touch(Current);
    }
    return model;
}

void Chat::copyChat()
{
    if (m_currentId.isEmpty())
        return;
    QGuiApplication::clipboard()->setText(ChatLogic::chatMarkdown(m_data.sessions.value(m_currentId),
                                                                  m_data.messages.value(m_currentId), false,
                                                                  QDateTime::currentDateTime(), now()));
    setNotice(QStringLiteral("Copied the chat as Markdown."));
}

void Chat::exportChat()
{
    if (m_currentId.isEmpty())
        return;
    const QJsonObject session = m_data.sessions.value(m_currentId);
    const QString markdown = ChatLogic::chatMarkdown(session, m_data.messages.value(m_currentId), false,
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
    const QStringList history = ChatLogic::promptHistory(m_data.messages.value(m_currentId), m_sent);
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
