#include "sessionlistmodel.h"

int SessionListModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : int(m_rows.size());
}

QVariant SessionListModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() >= m_rows.size())
        return {};
    const Row &row = m_rows[index.row()];
    switch (role) {
    case IdRole:
        return row.id;
    case TitleRole:
        return row.title;
    case UpdatedRole:
        return row.updated;
    case GroupRole:
        return row.group;
    case BusyRole:
        return row.busy;
    case WaitingRole:
        return row.waiting;
    }
    return {};
}

QHash<int, QByteArray> SessionListModel::roleNames() const
{
    return {{IdRole, "sessionId"}, {TitleRole, "title"},  {UpdatedRole, "updated"},
            {GroupRole, "group"},  {BusyRole, "busy"},     {WaitingRole, "waiting"}};
}

void SessionListModel::setQuery(const QString &query)
{
    if (query == m_query)
        return;
    m_query = query;
    emit queryChanged();
    apply();
}

void SessionListModel::update(const QList<Row> &all)
{
    m_all = all;
    apply();
}

void SessionListModel::apply()
{
    const QString needle = m_query.trimmed();
    QList<Row> next;
    for (const Row &row : m_all) {
        if (needle.isEmpty() || row.title.contains(needle, Qt::CaseInsensitive))
            next << row;
    }
    bool sameOrder = next.size() == m_rows.size();
    for (qsizetype i = 0; sameOrder && i < next.size(); ++i)
        sameOrder = next[i].id == m_rows[i].id;
    if (!sameOrder) {
        // a new order (a chat moved to the top): start over rather than animating every row
        const bool countChanges = next.size() != m_rows.size();
        beginResetModel();
        m_rows = next;
        endResetModel();
        if (countChanges)
            emit countChanged();
        return;
    }
    for (qsizetype i = 0; i < next.size(); ++i) {
        if (!(next[i] == m_rows[i])) {
            m_rows[i] = next[i];
            emit dataChanged(index(int(i)), index(int(i)));
        }
    }
}
