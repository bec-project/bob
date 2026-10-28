#include "plugins.h"

#include "bob.h"
#include "chat.h"
#include "mcpclient.h"
#include "prefs.h"


namespace {
Plugins *instance = nullptr;
}

McpServer::McpServer(const QString &name, const QString &url, QJSEngine *engine, Plugins *plugins)
    : QObject(plugins)
    , m_name(name)
    , m_url(url)
    , m_engine(engine)
    , m_plugins(plugins)
{
}

void McpServer::setToken(const QString &token)
{
    if (token == m_token)
        return;
    m_token = token;
    // a new token is a new session
    if (m_client)
        m_client->deleteLater();
    m_client = nullptr;
    emit tokenChanged();
}

McpClient *McpServer::client(QString *error)
{
    // only the servers of the bob this app holds, whatever connection the chat uses
    const QString url = m_plugins->heldUrl(m_name, error);
    if (url.isEmpty())
        return nullptr;
    if (m_client && m_clientUrl != url) {
        m_client->deleteLater();
        m_client = nullptr;
    }
    if (!m_client) {
        const auto target = McpClient::loopbackTarget(url, error);
        if (!target)
            return nullptr;
        m_client = new McpClient(*target, m_token, this);
        m_clientUrl = url;
    }
    return m_client;
}

void McpServer::answer(QJSValue callback, const QJsonValue &result, const QString &error)
{
    QJSEngine *engine = m_engine ? m_engine : qjsEngine(this);
    if (!callback.isCallable() || !engine)
        return;
    callback.call({engine->toScriptValue(result.toVariant()), QJSValue(error)});
}

void McpServer::call(const QString &tool, const QVariantMap &arguments, const QJSValue &callback, int timeoutMs)
{
    callTool(tool, QJsonObject::fromVariantMap(arguments), timeoutMs,
             [this, callback](const QJsonValue &result, const QString &error) { answer(callback, result, error); });
}

void McpServer::tools(const QJSValue &callback)
{
    listTools([this, callback](const QJsonValue &result, const QString &error) { answer(callback, result, error); });
}

void McpServer::callTool(const QString &tool, const QJsonObject &arguments, int timeoutMs, Callback done)
{
    QString error;
    McpClient *client = this->client(&error);
    if (!client) {
        done({}, error);
        return;
    }
    client->call(tool, arguments, timeoutMs, std::move(done));
}

void McpServer::listTools(Callback done)
{
    QString error;
    McpClient *client = this->client(&error);
    if (!client) {
        done({}, error);
        return;
    }
    client->tools(std::move(done));
}

void McpServer::initialize(std::function<void(const QJsonObject &, const QString &)> done)
{
    QString error;
    McpClient *client = this->client(&error);
    if (!client) {
        done({}, error);
        return;
    }
    client->initialize([done](const QJsonValue &info, const QString &error) { done(info.toObject(), error); });
}

Plugins::Plugins(Bob *bob, Chat *chat, Prefs *prefs, QObject *parent)
    : QObject(parent)
    , m_bob(bob)
    , m_chat(chat)
    , m_prefs(prefs)
{
    connect(m_chat, &Chat::phaseChanged, this, &Plugins::readConnection);
    connect(m_chat, &Chat::mcpChanged, this, &Plugins::readStatus);
    connect(m_prefs, &Prefs::changed, this, &Plugins::prefsChanged);
    readConnection();
}

Plugins *Plugins::create(QQmlEngine *engine, QJSEngine *)
{
    QJSEngine::setObjectOwnership(instance, QJSEngine::CppOwnership);
    instance->m_engine = engine;
    for (McpServer *server : std::as_const(instance->m_clients))
        server->setEngine(engine);
    return instance;
}

void Plugins::setInstance(Plugins *plugins)
{
    instance = plugins;
}

void Plugins::readConnection()
{
    const QVariantMap servers = m_chat->connection().value(QStringLiteral("mcpServers")).toMap();
    QVariantList list;
    QHash<QString, QString> urls;
    for (auto it = servers.cbegin(); it != servers.cend(); ++it) {
        const QVariantMap entry = it.value().toMap();
        const QString url = entry.value(QStringLiteral("url")).toString();
        if (url.isEmpty())
            continue;
        urls.insert(it.key(), url);
        list << QVariantMap{{QStringLiteral("name"), it.key()},
                            {QStringLiteral("url"), url},
                            {QStringLiteral("plugin"), entry.value(QStringLiteral("plugin")).toString()}};
    }
    if (urls == m_urls)
        return;
    for (auto it = m_clients.begin(); it != m_clients.end();) {
        if (urls.value(it.key()) != it.value()->url()) {
            it.value()->deleteLater();
            it = m_clients.erase(it);
        } else {
            ++it;
        }
    }
    m_urls = urls;
    m_servers = list;
    m_connected.clear();
    m_versions.clear();
    emit serversChanged();
    emit versionsChanged();
    readStatus();
}

void Plugins::readStatus()
{
    QSet<QString> connected;
    for (const QVariant &entry : m_chat->mcpServers()) {
        const QVariantMap server = entry.toMap();
        if (server.value(QStringLiteral("status")).toString() == QStringLiteral("connected"))
            connected.insert(server.value(QStringLiteral("name")).toString());
    }
    // a server that (re)connected may be a restarted or upgraded one: ask its version again
    for (const QString &name : connected) {
        if (!m_connected.contains(name) && m_urls.contains(name))
            probe(name);
    }
    m_connected = connected;
}

void Plugins::probe(const QString &name)
{
    McpServer *target = server(name);
    if (!target)
        return;
    QPointer<McpServer> guard(target);
    target->initialize([this, name, guard](const QJsonObject &info, const QString &error) {
        if (!guard || m_clients.value(name) != guard)
            return;
        const QString version = error.isEmpty() ? info.value(QStringLiteral("version")).toString() : QString();
        if (m_versions.value(name).toString() == version)
            return;
        if (version.isEmpty())
            m_versions.remove(name);
        else
            m_versions.insert(name, version);
        emit versionsChanged();
    });
}

McpServer *Plugins::server(const QString &name)
{
    if (!m_urls.contains(name))
        return nullptr;
    McpServer *server = m_clients.value(name);
    if (!server) {
        server = new McpServer(name, m_urls.value(name), m_engine, this);
        QJSEngine::setObjectOwnership(server, QJSEngine::CppOwnership);
        m_clients.insert(name, server);
    }
    return server;
}

QString Plugins::heldUrl(const QString &server, QString *error) const
{
    const BobStatus status = m_bob->bobStatus();
    if (status.state != BobStatus::Joined) {
        if (error)
            *error = tr("plugins call the MCP servers of the bob this app holds; join bob first");
        return {};
    }
    const QString url = status.connection.value(QStringLiteral("mcpServers")).toObject().value(server).toObject()
                            .value(QStringLiteral("url")).toString();
    if (url.isEmpty() && error)
        *error = tr("bob has no MCP server named %1").arg(server);
    return url;
}

bool Plugins::isEnabled(const QString &server) const
{
    return m_prefs->plugins().value(server, true).toBool();
}

void Plugins::setEnabled(const QString &server, bool enabled)
{
    QVariantMap switches = m_prefs->plugins();
    switches.insert(server, enabled);
    m_prefs->setPlugins(switches);
}

QVariantMap Plugins::settings(const QString &server) const
{
    return m_prefs->pluginSettings().value(server).toMap();
}

void Plugins::setSettings(const QString &server, const QVariantMap &patch)
{
    QVariantMap all = m_prefs->pluginSettings();
    QVariantMap mine = all.value(server).toMap();
    for (auto it = patch.cbegin(); it != patch.cend(); ++it)
        mine.insert(it.key(), it.value());
    all.insert(server, mine);
    m_prefs->setPluginSettings(all);
}
