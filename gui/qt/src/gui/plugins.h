#pragma once

#include <QHash>
#include <QJSValue>
#include <QObject>
#include <QPointer>
#include <QQmlEngine>
#include <QVariantMap>

#include <functional>

class Bob;
class Chat;
class McpClient;
class Plugins;
class Prefs;

// One MCP server of bob.toml, as the plugins call it: its tools, with this window's bearer token.
class McpServer : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Plugins provides the servers")
    Q_PROPERTY(QString name READ name CONSTANT)
    Q_PROPERTY(QString url READ url CONSTANT)
    // A bearer token for a server started with --token-file, kept for this window only
    Q_PROPERTY(QString token READ token WRITE setToken NOTIFY tokenChanged)

public:
    McpServer(const QString &name, const QString &url, QJSEngine *engine, Plugins *plugins);

    void setEngine(QJSEngine *engine) { m_engine = engine; }
    QString name() const { return m_name; }
    QString url() const { return m_url; }
    QString token() const { return m_token; }
    void setToken(const QString &token);

    // Calls a tool; `callback(result, error)` gets its structured output (or its text, parsed as
    // JSON when it is) or the error. The call gives up after timeoutMs, 2 minutes when 0.
    Q_INVOKABLE void call(const QString &tool, const QVariantMap &arguments, const QJSValue &callback,
                          int timeoutMs = 0);
    // `callback(tools, error)` gets the server's tools: [{name, description}, ...].
    Q_INVOKABLE void tools(const QJSValue &callback);
    // The same two calls for plugins' C++, with the answers as JSON.
    using Callback = std::function<void(const QJsonValue &result, const QString &error)>;
    void callTool(const QString &tool, const QJsonObject &arguments, int timeoutMs, Callback done);
    void listTools(Callback done);

signals:
    void tokenChanged();

private:
    McpClient *client(QString *error);
    void answer(QJSValue callback, const QJsonValue &result, const QString &error);

    QString m_name;
    QString m_url;
    QString m_token;
    QJSEngine *m_engine;
    Plugins *m_plugins;
    QPointer<McpClient> m_client;
    // where m_client calls: the server's URL in the held bob's connection
    QString m_clientUrl;
};

// The plugins' side of the app: the MCP servers bob.toml names (with the plugin that serves each),
// a client for each, and their switches and settings. The servers come from the chat's
// connection, so their views stay while bob is lost; their calls reach only the servers of the bob
// this app holds.
class Plugins : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    // [{name, url, plugin}] for every MCP server of the held bob that has a URL, by name
    Q_PROPERTY(QVariantList servers READ servers NOTIFY serversChanged)

public:
    Plugins(Bob *bob, Chat *chat, Prefs *prefs, QObject *parent = nullptr);

    static Plugins *create(QQmlEngine *engine, QJSEngine *);
    static void setInstance(Plugins *plugins);

    QVariantList servers() const { return m_servers; }

    // The server named `name` in bob.toml, for a plugin instance's calls; null when there is none.
    Q_INVOKABLE McpServer *server(const QString &name);
    // Whether the plugin instance serving `server` is switched on (it is unless switched off).
    Q_INVOKABLE bool isEnabled(const QString &server) const;
    Q_INVOKABLE void setEnabled(const QString &server, bool enabled);
    // An instance's own settings.
    Q_INVOKABLE QVariantMap settings(const QString &server) const;
    Q_INVOKABLE void setSettings(const QString &server, const QVariantMap &patch);
    // Where this machine reaches `server` in the connection of the bob this app holds; empty, with
    // the reason in `error`, while the app holds none or that bob has no such server.
    QString heldUrl(const QString &server, QString *error) const;

signals:
    void serversChanged();
    // A switch or an instance's settings changed.
    void prefsChanged();

private:
    void readConnection();

    Bob *m_bob;
    Chat *m_chat;
    Prefs *m_prefs;
    QJSEngine *m_engine = nullptr;
    QVariantList m_servers;
    QHash<QString, QString> m_urls;
    QHash<QString, McpServer *> m_clients;
};
