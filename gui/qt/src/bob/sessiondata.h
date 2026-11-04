#pragma once

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QList>
#include <QSet>

// One message with its parts, as GET /session/{id}/message returns it.
struct MessageEntry
{
    QJsonObject info;
    QList<QJsonObject> parts;

    QString id() const { return info.value(QStringLiteral("id")).toString(); }
    QString role() const { return info.value(QStringLiteral("role")).toString(); }
};

// The workspace changes one prompt led to (OpenCode diffs the snapshots around it).
struct PromptChanges
{
    QString messageID;
    QString prompt;
    QJsonArray files;
};

// Everything the event stream keeps current, keyed by id.
struct SessionData
{
    QHash<QString, QJsonObject> sessions;
    QHash<QString, QJsonObject> status;
    // Messages of the sessions that were opened, oldest first
    QHash<QString, QList<MessageEntry>> messages;
    QHash<QString, QJsonObject> permissions;
    QHash<QString, QJsonObject> questions;
    // The last error a session reported, until its next prompt
    QHash<QString, QString> errors;
    // What each prompt of a chat changed in the workspace, oldest first
    QHash<QString, QList<PromptChanges>> changes;
    // The agent's todo list per chat
    QHash<QString, QJsonArray> todos;
};

// What one event changed, so only those views are refreshed.
struct EventEffect
{
    enum Area {
        None = 0,
        Sessions = 1,
        Status = 2,
        Messages = 4,
        Requests = 8,
        Errors = 16,
        Todos = 32,
        Changes = 64,
    };
    int areas = None;
    // The session whose messages, status, error or todos changed
    QString sessionID;

    bool changed() const { return areas != None; }
};

// Applies one server event.
EventEffect applyEvent(SessionData &data, const QJsonObject &event);

// What an error object says, for the user.
QString errorText(const QJsonValue &error);

// Sessions newest first.
QList<QJsonObject> sortedSessions(const QHash<QString, QJsonObject> &sessions);

// The text the user typed in a message, without parts OpenCode added.
QString promptText(const MessageEntry &entry, const QString &separator = QStringLiteral("\n"));
