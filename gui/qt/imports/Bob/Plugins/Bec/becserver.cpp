#include "becserver.h"

#include "beccards.h"
#include "plugins.h"

#include <QJsonArray>
#include <QPointer>

void BecServer::answer(QJSValue done, const QVariantList &arguments)
{
    QJSEngine *engine = qjsEngine(this);
    if (!done.isCallable() || !engine)
        return;
    QJSValueList values;
    for (const QVariant &argument : arguments)
        values << engine->toScriptValue(argument);
    done.call(values);
}

void BecServer::callResult(QObject *server, const QString &tool, const QVariantMap &arguments, const QJSValue &done)
{
    auto *mcp = qobject_cast<McpServer *>(server);
    if (!mcp) {
        answer(done, {QVariant(), tr("no BEC server")});
        return;
    }
    QPointer<BecServer> self(this);
    mcp->callTool(tool, QJsonObject::fromVariantMap(arguments), 0, [self, tool, done](const QJsonValue &value, const QString &error) {
        if (!self)
            return;
        if (!error.isEmpty()) {
            self->answer(done, {QVariant(), error});
            return;
        }
        const QJsonObject result = BecCards::parseToolResult(value);
        if (result.isEmpty())
            self->answer(done, {QVariant(), tr("%1 returned an unexpected result").arg(tool)});
        else
            self->answer(done, {result.toVariantMap(), QString()});
    });
}

void BecServer::tools(QObject *server, const QJSValue &done)
{
    auto *mcp = qobject_cast<McpServer *>(server);
    if (!mcp) {
        answer(done, {QVariantMap(), tr("no BEC server")});
        return;
    }
    const QString key = mcp->name() + QLatin1Char('\n') + mcp->token();
    if (m_tools.contains(key)) {
        answer(done, {m_tools.value(key), QString()});
        return;
    }
    QPointer<BecServer> self(this);
    mcp->listTools([self, key, done](const QJsonValue &list, const QString &error) {
        if (!self)
            return;
        if (!error.isEmpty()) {
            self->answer(done, {QVariantMap(), error});
            return;
        }
        QVariantMap names;
        for (const QJsonValue &tool : list.toArray())
            names.insert(tool.toObject().value(QLatin1String("name")).toString(), true);
        self->m_tools.insert(key, names);
        self->answer(done, {names, QString()});
    });
}
