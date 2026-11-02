#pragma once

#include <QHash>
#include <QJSValue>
#include <QObject>
#include <QQmlEngine>
#include <QVariantMap>

// Calls to the BEC MCP server a plugin instance serves: its McpServer (context.server), with this
// window's token.
class BecServer : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON

public:
    using QObject::QObject;

    // Calls a BEC tool; `done(result, error)` gets its tool_result() object, or the error.
    Q_INVOKABLE void callResult(QObject *server, const QString &tool, const QVariantMap &arguments, const QJSValue &done);
    // `done(names, error)` gets the server's tool names as an object used as a set ({name: true}),
    // asked once per server and token; {} and why when the server cannot say, which is asked again
    // next time.
    Q_INVOKABLE void tools(QObject *server, const QJSValue &done);

private:
    void answer(QJSValue done, const QVariantList &arguments);

    // server name and token -> its tool names
    QHash<QString, QVariantMap> m_tools;
};
