#pragma once

#include <QAbstractListModel>
#include <QJsonObject>
#include <QQmlEngine>

// The chat list: root sessions newest first, grouped by day (the `group` role, for sections),
// filtered by the search text.
class SessionListModel : public QAbstractListModel
{
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Chat provides the chat list")
    Q_PROPERTY(QString query READ query WRITE setQuery NOTIFY queryChanged)
    Q_PROPERTY(int count READ count NOTIFY countChanged)

public:
    enum Role { IdRole = Qt::UserRole + 1, TitleRole, UpdatedRole, GroupRole, BusyRole, WaitingRole };

    struct Row
    {
        QString id;
        QString title;
        double updated = 0;
        QString group;
        bool busy = false;
        // a permission or question waits for the user
        bool waiting = false;

        bool operator==(const Row &other) const
        {
            return id == other.id && title == other.title && updated == other.updated && group == other.group
                && busy == other.busy && waiting == other.waiting;
        }
    };

    using QAbstractListModel::QAbstractListModel;

    int rowCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    int count() const { return int(m_rows.size()); }

    QString query() const { return m_query; }
    void setQuery(const QString &query);

    // Every session, newest first; the model keeps those matching the query.
    void update(const QList<Row> &all);

signals:
    void queryChanged();
    void countChanged();

private:
    void apply();

    QString m_query;
    QList<Row> m_all;
    QList<Row> m_rows;
};
