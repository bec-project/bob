#include "sessiondata.h"

#include <algorithm>

namespace {

QString str(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toString();
}

// OpenCode ids grow with time within a session, so sorting by id keeps the server's order.
bool byId(const QJsonObject &a, const QJsonObject &b)
{
    return str(a, "id") < str(b, "id");
}

qsizetype indexOfMessage(const QList<MessageEntry> &list, const QString &id)
{
    for (qsizetype i = 0; i < list.size(); ++i) {
        if (list[i].id() == id)
            return i;
    }
    return -1;
}

qsizetype indexOfPart(const QList<QJsonObject> &parts, const QString &id)
{
    for (qsizetype i = 0; i < parts.size(); ++i) {
        if (str(parts[i], "id") == id)
            return i;
    }
    return -1;
}

}  // namespace

QString errorText(const QJsonValue &error)
{
    if (!error.isObject())
        return error.isNull() || error.isUndefined() ? QStringLiteral("Unknown error") : error.toVariant().toString();
    const QJsonObject value = error.toObject();
    if (str(value, "name") == QStringLiteral("MessageAbortedError"))
        return QStringLiteral("Stopped.");
    QString message = value.value(QStringLiteral("data")).toObject().value(QStringLiteral("message")).toString();
    if (message.isEmpty())
        message = str(value, "message");
    if (!message.isEmpty())
        return message;
    const QString name = str(value, "name");
    return name.isEmpty() ? QStringLiteral("Unknown error") : name;
}

QList<QJsonObject> sortedSessions(const QHash<QString, QJsonObject> &sessions)
{
    QList<QJsonObject> list = sessions.values();
    auto updated = [](const QJsonObject &session) {
        return session.value(QStringLiteral("time")).toObject().value(QStringLiteral("updated")).toDouble();
    };
    std::sort(list.begin(), list.end(), [&](const QJsonObject &a, const QJsonObject &b) {
        const double ua = updated(a), ub = updated(b);
        return ua != ub ? ua > ub : str(a, "id") > str(b, "id");
    });
    return list;
}

QString promptText(const MessageEntry &entry, const QString &separator)
{
    QStringList texts;
    for (const QJsonObject &part : entry.parts) {
        if (str(part, "type") == QStringLiteral("text") && !part.value(QStringLiteral("synthetic")).toBool())
            texts << str(part, "text");
    }
    return texts.join(separator);
}

EventEffect applyEvent(SessionData &data, const QJsonObject &event)
{
    const QString type = str(event, "type");
    const QJsonObject properties = event.value(QStringLiteral("properties")).toObject();
    EventEffect effect;

    if (type == QStringLiteral("session.created") || type == QStringLiteral("session.updated")) {
        const QJsonObject info = properties.value(QStringLiteral("info")).toObject();
        if (!str(info, "parentID").isEmpty())
            return effect;
        const QString id = str(info, "id");
        data.sessions.insert(id, info);
        effect.areas = EventEffect::Sessions;
        effect.sessionID = id;
    } else if (type == QStringLiteral("session.deleted")) {
        const QString id = str(properties.value(QStringLiteral("info")).toObject(), "id");
        data.sessions.remove(id);
        data.messages.remove(id);
        data.status.remove(id);
        data.changes.remove(id);
        data.todos.remove(id);
        effect.areas = EventEffect::Sessions | EventEffect::Messages | EventEffect::Status | EventEffect::Changes
            | EventEffect::Todos;
        effect.sessionID = id;
    } else if (type == QStringLiteral("session.status")) {
        effect.sessionID = str(properties, "sessionID");
        data.status.insert(effect.sessionID, properties.value(QStringLiteral("status")).toObject());
        effect.areas = EventEffect::Status;
    } else if (type == QStringLiteral("session.idle")) {
        effect.sessionID = str(properties, "sessionID");
        data.status.insert(effect.sessionID, QJsonObject{{QStringLiteral("type"), QStringLiteral("idle")}});
        effect.areas = EventEffect::Status;
    } else if (type == QStringLiteral("session.error")) {
        const QString sessionID = str(properties, "sessionID");
        const QJsonValue error = properties.value(QStringLiteral("error"));
        if (sessionID.isEmpty() || error.isUndefined() || error.isNull())
            return effect;
        data.errors.insert(sessionID, errorText(error));
        effect.areas = EventEffect::Errors;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("message.updated")) {
        const QJsonObject info = properties.value(QStringLiteral("info")).toObject();
        const QString sessionID = str(info, "sessionID");
        auto list = data.messages.find(sessionID);
        if (list == data.messages.end())
            return effect;
        const qsizetype index = indexOfMessage(*list, str(info, "id"));
        if (index == -1) {
            list->append({info, {}});
            std::stable_sort(list->begin(), list->end(),
                             [](const MessageEntry &a, const MessageEntry &b) { return a.id() < b.id(); });
        } else {
            (*list)[index].info = info;
        }
        effect.areas = EventEffect::Messages;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("message.removed")) {
        const QString sessionID = str(properties, "sessionID");
        auto list = data.messages.find(sessionID);
        if (list == data.messages.end())
            return effect;
        const qsizetype index = indexOfMessage(*list, str(properties, "messageID"));
        if (index == -1)
            return effect;
        list->removeAt(index);
        effect.areas = EventEffect::Messages;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("message.part.updated")) {
        const QJsonObject part = properties.value(QStringLiteral("part")).toObject();
        const QString sessionID = str(part, "sessionID");
        auto list = data.messages.find(sessionID);
        if (list == data.messages.end())
            return effect;
        // a part that arrives before its message waits for message.updated, which carries it again
        const qsizetype index = indexOfMessage(*list, str(part, "messageID"));
        if (index == -1)
            return effect;
        QList<QJsonObject> &parts = (*list)[index].parts;
        const qsizetype partIndex = indexOfPart(parts, str(part, "id"));
        if (partIndex == -1) {
            parts.append(part);
            std::stable_sort(parts.begin(), parts.end(), byId);
        } else {
            parts[partIndex] = part;
        }
        effect.areas = EventEffect::Messages;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("message.part.delta")) {
        const QString sessionID = str(properties, "sessionID");
        auto list = data.messages.find(sessionID);
        if (list == data.messages.end())
            return effect;
        const qsizetype index = indexOfMessage(*list, str(properties, "messageID"));
        if (index == -1)
            return effect;
        QList<QJsonObject> &parts = (*list)[index].parts;
        const qsizetype partIndex = indexOfPart(parts, str(properties, "partID"));
        if (partIndex == -1)
            return effect;
        const QString field = str(properties, "field");
        const QJsonValue current = parts[partIndex].value(field);
        if (!current.isString() && !current.isUndefined())
            return effect;
        parts[partIndex].insert(field, current.toString() + str(properties, "delta"));
        effect.areas = EventEffect::Messages;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("message.part.removed")) {
        const QString sessionID = str(properties, "sessionID");
        auto list = data.messages.find(sessionID);
        if (list == data.messages.end())
            return effect;
        const qsizetype index = indexOfMessage(*list, str(properties, "messageID"));
        if (index == -1)
            return effect;
        QList<QJsonObject> &parts = (*list)[index].parts;
        const qsizetype partIndex = indexOfPart(parts, str(properties, "partID"));
        if (partIndex == -1)
            return effect;
        parts.removeAt(partIndex);
        effect.areas = EventEffect::Messages;
        effect.sessionID = sessionID;
    } else if (type == QStringLiteral("permission.asked")) {
        data.permissions.insert(str(properties, "id"), properties);
        effect.areas = EventEffect::Requests;
        effect.sessionID = str(properties, "sessionID");
    } else if (type == QStringLiteral("permission.replied")) {
        if (data.permissions.remove(str(properties, "requestID")) == 0)
            return effect;
        effect.areas = EventEffect::Requests;
        effect.sessionID = str(properties, "sessionID");
    } else if (type == QStringLiteral("question.asked")) {
        data.questions.insert(str(properties, "id"), properties);
        effect.areas = EventEffect::Requests;
        effect.sessionID = str(properties, "sessionID");
    } else if (type == QStringLiteral("question.replied") || type == QStringLiteral("question.rejected")) {
        if (data.questions.remove(str(properties, "requestID")) == 0)
            return effect;
        effect.areas = EventEffect::Requests;
        effect.sessionID = str(properties, "sessionID");
    } else if (type == QStringLiteral("todo.updated")) {
        effect.sessionID = str(properties, "sessionID");
        data.todos.insert(effect.sessionID, properties.value(QStringLiteral("todos")).toArray());
        effect.areas = EventEffect::Todos;
    }
    return effect;
}
