#pragma once

#include "sessiondata.h"

#include <QAbstractListModel>
#include <QQmlEngine>
#include <QVariantMap>

// A chat as a flat list of rows for a ListView: the user's prompt, each part of the answers
// (thinking, Markdown and code blocks, tool calls, ...), errors, a working indicator and each
// turn's footer. Rows have stable keys, so an update only touches the rows that changed: a
// streaming answer changes its last row, and delegates keep their state.
//
// A sub-agent's run uses the same model in steps mode: only the parts of its assistant messages.
class ThreadModel : public QAbstractListModel
{
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Chat provides the thread models")
    Q_PROPERTY(int count READ count NOTIFY countChanged)

public:
    enum Role { KeyRole = Qt::UserRole + 1, KindRole, ItemRole };

    struct Row
    {
        QString key;
        QString kind;
        QVariantMap item;
    };

    // What a session looks like now.
    struct Input
    {
        const QList<MessageEntry> *entries = nullptr;
        bool loading = false;
        bool busy = false;
        // The session's pending permission requests
        QList<QJsonObject> permissions;
        // The first message an undo removed
        QString revertID;
        // The session's last error, until its next prompt
        QString sessionError;
        // The session's status: idle, busy or retry (with attempt and message)
        QJsonObject status;
        // Rows the user opened or closed, by key
        const QHash<QString, bool> *expanded = nullptr;
    };

    explicit ThreadModel(bool steps, QObject *parent = nullptr);

    int rowCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    int count() const { return int(m_rows.size()); }

    void update(const Input &input);
    // Starts over, for another session: the view resets instead of animating every row.
    void reset();
    const QList<Row> &rows() const { return m_rows; }

signals:
    void countChanged();

private:
    QList<Row> build(const Input &input);
    void addStep(QList<Row> &rows, const Input &input, const MessageEntry &step, bool live, bool undone);
    void addPart(QList<Row> &rows, const Input &input, const QJsonObject &part, bool live, bool undone);

    struct ToolText
    {
        QJsonObject state;
        QString input;
        QString output;
    };

    bool m_steps;
    QList<Row> m_rows;
    // Tool inputs and outputs made readable, by part id, while their state stays the same
    QHash<QString, ToolText> m_toolText;
};
