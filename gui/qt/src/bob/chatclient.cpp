#include "chatclient.h"

#include "eventstream.h"
#include "turns.h"

#include <memory>

namespace {

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

ChatClient::ChatClient(QObject *parent)
    : QObject(parent)
{
    m_mcpTimer.setInterval(mcpPollMs);
    connect(&m_mcpTimer, &QTimer::timeout, this, &ChatClient::refreshMcp);
}

ChatClient::~ChatClient()
{
    stop();
}

QJsonObject ChatClient::model(const ModelRef &ref) const
{
    if (!ref.isValid())
        return {};
    for (const QJsonValue &provider : m_providers) {
        if (str(provider.toObject(), "id") == ref.providerID)
            return obj(obj(provider.toObject(), "models"), ref.modelID.toUtf8().constData());
    }
    return {};
}

ApiCallback ChatClient::guarded(std::function<void(const QJsonValue &, const std::optional<ApiError> &)> done)
{
    OpencodeApi *api = m_api;
    return [this, api, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (current(api))
            done(value, error);
    };
}

// ---------- the connection ----------

void ChatClient::stop()
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
    qDeleteAll(m_resync);
    m_resync.clear();
}

void ChatClient::close()
{
    stop();
    m_data = SessionData();
    m_loading.clear();
    m_connection = {};
    m_focus.clear();
    m_stream = QStringLiteral("closed");
    m_streamDetail.clear();
    emit streamChanged();
}

void ChatClient::open(const QJsonObject &connection)
{
    stop();
    auto *api = new OpencodeApi(connection, this);
    m_api = api;
    m_connection = connection;
    m_data = SessionData();
    m_loading.clear();
    m_focus.clear();

    auto giveUp = [this, api](const QString &error) {
        if (!current(api))
            return;
        stop();
        m_connection = {};
        emit openFailed(error);
    };

    api->health([this, api, giveUp](const QJsonValue &health, const std::optional<ApiError> &healthError) {
        if (!current(api))
            return;
        // Older servers have no /global/health; the session list proves the password.
        if (healthError && healthError->status != 404) {
            giveUp(healthError->message);
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
        gather->done = [this, api, results, giveUp] {
            if (!current(api))
                return;
            if (!results->error.isEmpty()) {
                giveUp(results->error);
                return;
            }
            const QJsonObject providers = results->providers.toObject();
            const QJsonObject config = results->config.toObject();
            m_providers = providers.value(QStringLiteral("providers")).toArray();
            m_defaultModel = ModelRef::fromKey(str(config, "model"));
            const QJsonObject defaults = obj(providers, "default");
            if (!m_defaultModel.isValid() && !defaults.isEmpty())
                m_defaultModel = {defaults.begin().key(), defaults.begin().value().toString()};
            m_agents = results->agents.toArray();
            m_defaultAgent = str(config, "default_agent").isEmpty() ? QStringLiteral("build") : str(config, "default_agent");
            m_mcpStatus = results->mcp.toObject();
            emit opened();
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
                        emit failed({0, error});
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
                close();
                emit unauthorized();
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

void ChatClient::refreshLists(std::function<void(bool, QString)> done)
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
        emit changed(QString(), EventEffect::Sessions | EventEffect::Status | EventEffect::Requests);
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

void ChatClient::refreshMcp()
{
    if (!m_api)
        return;
    m_api->mcpStatus(guarded([this](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (error || value.toObject() == m_mcpStatus)
            return;
        m_mcpStatus = value.toObject();
        emit mcpChanged();
    }));
}

void ChatClient::onEvent(const QJsonObject &event)
{
    const QString type = str(event, "type");
    if (type == QStringLiteral("mcp.tools.changed"))
        refreshMcp();
    const EventEffect effect = applyEvent(m_data, event);
    if (effect.changed())
        emit changed(effect.sessionID, effect.areas);
    const QJsonObject properties = event.value(QStringLiteral("properties")).toObject();
    if (type == QStringLiteral("session.idle"))
        resync(str(properties, "sessionID"));
    if (type == QStringLiteral("session.deleted"))
        emit sessionDeleted(str(obj(properties, "info"), "id"));
}

// ---------- loading a session ----------

void ChatClient::loadMessages(const QString &sessionID)
{
    if (!m_api || sessionID.isEmpty())
        return;
    m_loading.insert(sessionID, true);
    emit changed(sessionID, EventEffect::Messages);
    m_api->messages(sessionID, guarded([this, sessionID](const QJsonValue &value, const std::optional<ApiError> &error) {
        m_loading.insert(sessionID, false);
        if (error) {
            emit failed(*error);
        } else {
            m_data.messages.insert(sessionID, parseEntries(value));
            if (sessionID == m_focus)
                loadChanges(sessionID);
        }
        // a pending request shows its tool call, which these messages may hold
        emit changed(sessionID, EventEffect::Messages | EventEffect::Requests);
    }));
}

// The session-wide diff stays empty in OpenCode 1.18, so changes are asked per prompt, and only for
// the prompts whose answers report changed files (a patch part).
void ChatClient::loadChanges(const QString &sessionID)
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
        emit changed(sessionID, EventEffect::Changes);
    };
    for (int i = 0; i < prompts.size(); ++i) {
        api->diff(sessionID, prompts[i].messageID, [collected, gather, i](const QJsonValue &value, const std::optional<ApiError> &) {
            (*collected)[i].files = value.toArray();
            gather->finish();
        });
    }
    gather->finish();
}

void ChatClient::loadExtras(const QString &sessionID)
{
    if (!m_api || sessionID.isEmpty())
        return;
    m_api->todos(sessionID, guarded([this, sessionID](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.todos.insert(sessionID, value.toArray());
            emit changed(sessionID, EventEffect::Todos);
        }
        loadChanges(sessionID);
    }));
}

// After a turn ends, fetch the session once more so a missed event cannot leave it stale.
void ChatClient::resync(const QString &sessionID)
{
    QTimer *timer = m_resync.value(sessionID);
    if (!timer) {
        timer = new QTimer(this);
        timer->setSingleShot(true);
        timer->setInterval(300);
        connect(timer, &QTimer::timeout, this, [this, sessionID] {
            if (m_data.messages.contains(sessionID))
                loadMessages(sessionID);
            if (sessionID == m_focus)
                loadExtras(sessionID);
        });
        m_resync.insert(sessionID, timer);
    }
    timer->start();
}

// ---------- requests ----------

bool ChatClient::createSession(Created done)
{
    if (!m_api)
        return false;
    m_api->createSession(guarded([this, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (error) {
            done({}, error);
            return;
        }
        const QJsonObject session = value.toObject();
        const QString id = str(session, "id");
        m_data.sessions.insert(id, session);
        m_data.messages.insert(id, {});
        emit changed(id, EventEffect::Sessions | EventEffect::Messages);
        done(id, std::nullopt);
    }));
    return true;
}

bool ChatClient::prompt(const QString &sessionID, const QString &text, const ModelRef &model, const QString &agent,
                        const QString &variant, Done done)
{
    if (!m_api)
        return false;
    m_data.errors.remove(sessionID);
    emit changed(sessionID, EventEffect::Errors);
    m_api->prompt(sessionID, text, model, agent, variant,
                  guarded([done](const QJsonValue &, const std::optional<ApiError> &error) { done(error); }));
    return true;
}

bool ChatClient::abort(const QString &sessionID, Done done)
{
    if (!m_api)
        return false;
    m_api->abort(sessionID, guarded([done](const QJsonValue &, const std::optional<ApiError> &error) { done(error); }));
    return true;
}

bool ChatClient::renameSession(const QString &sessionID, const QString &title, Done done)
{
    if (!m_api)
        return false;
    m_api->renameSession(sessionID, title, guarded([this, sessionID, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.sessions.insert(sessionID, value.toObject());
            emit changed(sessionID, EventEffect::Sessions);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::deleteSession(const QString &sessionID, Done done)
{
    if (!m_api)
        return false;
    m_api->deleteSession(sessionID, guarded([this, sessionID, done](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.sessions.remove(sessionID);
            m_data.messages.remove(sessionID);
            emit changed(sessionID, EventEffect::Sessions);
            emit sessionDeleted(sessionID);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::replyPermission(const QString &id, const QString &reply, Done done)
{
    if (!m_api)
        return false;
    m_api->replyPermission(id, reply, guarded([this, id, done](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.permissions.remove(id);
            emit changed(QString(), EventEffect::Requests);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::replyQuestion(const QString &id, const QJsonArray &answers, Done done)
{
    if (!m_api)
        return false;
    m_api->replyQuestion(id, answers, guarded([this, id, done](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.questions.remove(id);
            emit changed(QString(), EventEffect::Requests);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::rejectQuestion(const QString &id, Done done)
{
    if (!m_api)
        return false;
    m_api->rejectQuestion(id, guarded([this, id, done](const QJsonValue &, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.questions.remove(id);
            emit changed(QString(), EventEffect::Requests);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::compact(const QString &sessionID, const ModelRef &model, Done done)
{
    if (!m_api)
        return false;
    m_api->compact(sessionID, model, guarded([done](const QJsonValue &, const std::optional<ApiError> &error) { done(error); }));
    return true;
}

bool ChatClient::revert(const QString &sessionID, const QString &messageID, Done done)
{
    if (!m_api)
        return false;
    m_api->revert(sessionID, messageID, guarded([this, sessionID, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.sessions.insert(sessionID, value.toObject());
            loadExtras(sessionID);
            emit changed(sessionID, EventEffect::Sessions | EventEffect::Changes);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::unrevert(const QString &sessionID, Done done)
{
    if (!m_api)
        return false;
    m_api->unrevert(sessionID, guarded([this, sessionID, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (!error) {
            m_data.sessions.insert(sessionID, value.toObject());
            loadExtras(sessionID);
            emit changed(sessionID, EventEffect::Sessions | EventEffect::Changes);
        }
        done(error);
    }));
    return true;
}

bool ChatClient::fork(const QString &sessionID, const QString &messageID, Created done)
{
    if (!m_api)
        return false;
    m_api->fork(sessionID, messageID, guarded([this, done](const QJsonValue &value, const std::optional<ApiError> &error) {
        if (error) {
            done({}, error);
            return;
        }
        const QJsonObject session = value.toObject();
        const QString id = str(session, "id");
        m_data.sessions.insert(id, session);
        emit changed(id, EventEffect::Sessions);
        done(id, std::nullopt);
    }));
    return true;
}

bool ChatClient::reconnectMcp(const QString &name, Done done)
{
    if (!m_api)
        return false;
    QJsonObject entry = m_mcpStatus.value(name).toObject();
    entry.insert(QStringLiteral("status"), QStringLiteral("connecting"));
    entry.remove(QStringLiteral("error"));
    m_mcpStatus.insert(name, entry);
    emit mcpChanged();
    m_api->mcpConnect(name, guarded([this, done](const QJsonValue &, const std::optional<ApiError> &error) {
        done(error);
        refreshMcp();
    }));
    return true;
}
