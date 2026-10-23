#include "threadmodel.h"

#include "chatlogic.h"
#include "turns.h"

#include <QDateTime>

namespace {

const int outputLimit = 20000;

QString str(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toString();
}

QJsonObject obj(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toObject();
}

double num(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toDouble();
}

bool hasError(const QJsonObject &info)
{
    const QJsonValue error = info.value(QStringLiteral("error"));
    return !error.isUndefined() && !error.isNull();
}

QString userText(const MessageEntry &entry)
{
    return promptText(entry);
}

QString answerText(const QList<MessageEntry> &entries, const Turn &turn)
{
    QStringList texts;
    for (int index : turn.steps) {
        for (const QJsonObject &part : entries[index].parts) {
            if (str(part, "type") != QStringLiteral("text") || part.value(QStringLiteral("synthetic")).toBool())
                continue;
            const QString text = str(part, "text").trimmed();
            if (!text.isEmpty())
                texts << text;
        }
    }
    return texts.join(QStringLiteral("\n\n"));
}

ThreadModel::Row row(const QString &key, const QString &kind, QVariantMap item = {}, bool undone = false)
{
    item.insert(QStringLiteral("undone"), undone);
    return {key, kind, item};
}

}  // namespace

ThreadModel::ThreadModel(bool steps, QObject *parent)
    : QAbstractListModel(parent)
    , m_steps(steps)
{
}

int ThreadModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : int(m_rows.size());
}

QVariant ThreadModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() >= m_rows.size())
        return {};
    const Row &row = m_rows[index.row()];
    switch (role) {
    case KeyRole:
        return row.key;
    case KindRole:
        return row.kind;
    case ItemRole:
        return row.item;
    }
    return {};
}

QHash<int, QByteArray> ThreadModel::roleNames() const
{
    return {{KeyRole, "key"}, {KindRole, "kind"}, {ItemRole, "item"}};
}

void ThreadModel::reset()
{
    beginResetModel();
    m_rows.clear();
    m_toolText.clear();
    endResetModel();
    emit countChanged();
}

void ThreadModel::update(const Input &input)
{
    const QList<Row> next = build(input);
    const qsizetype oldCount = m_rows.size();
    const qsizetype newCount = next.size();
    qsizetype prefix = 0;
    while (prefix < oldCount && prefix < newCount && m_rows[prefix].key == next[prefix].key)
        ++prefix;
    qsizetype suffix = 0;
    while (suffix < oldCount - prefix && suffix < newCount - prefix
           && m_rows[oldCount - 1 - suffix].key == next[newCount - 1 - suffix].key) {
        ++suffix;
    }
    auto changed = [](const Row &a, const Row &b) { return a.kind != b.kind || a.item != b.item; };
    for (qsizetype i = 0; i < prefix; ++i) {
        if (changed(m_rows[i], next[i])) {
            m_rows[i] = next[i];
            emit dataChanged(index(int(i)), index(int(i)));
        }
    }
    if (oldCount - prefix - suffix > 0) {
        beginRemoveRows({}, int(prefix), int(oldCount - suffix - 1));
        m_rows.remove(prefix, oldCount - prefix - suffix);
        endRemoveRows();
    }
    if (newCount - prefix - suffix > 0) {
        beginInsertRows({}, int(prefix), int(newCount - suffix - 1));
        for (qsizetype i = prefix; i < newCount - suffix; ++i)
            m_rows.insert(i, next[i]);
        endInsertRows();
    }
    for (qsizetype i = newCount - suffix; i < newCount; ++i) {
        if (changed(m_rows[i], next[i])) {
            m_rows[i] = next[i];
            emit dataChanged(index(int(i)), index(int(i)));
        }
    }
    if (oldCount != newCount)
        emit countChanged();
}

QList<ThreadModel::Row> ThreadModel::build(const Input &input)
{
    QList<Row> rows;
    if (!input.entries) {
        if (input.loading || m_steps) {
            const QString text = m_steps ? QStringLiteral("Loading the sub-agent's work…") : QStringLiteral("Loading the chat…");
            rows << row(QStringLiteral("loading"), QStringLiteral("loading"), {{QStringLiteral("text"), text}});
        } else {
            rows << row(QStringLiteral("empty"), QStringLiteral("empty"));
        }
        return rows;
    }
    const QList<MessageEntry> &entries = *input.entries;

    if (m_steps) {
        QList<int> steps;
        for (int i = 0; i < entries.size(); ++i) {
            if (entries[i].role() == QStringLiteral("assistant"))
                steps << i;
        }
        for (int i = 0; i < steps.size(); ++i)
            addStep(rows, input, entries[steps[i]], input.busy && i == steps.size() - 1, false);
        if (steps.isEmpty())
            rows << row(QStringLiteral("note"), QStringLiteral("note"), {{QStringLiteral("text"), QStringLiteral("No steps yet.")}});
        return rows;
    }

    const QList<Turn> turns = groupTurns(entries);
    const double now = double(QDateTime::currentMSecsSinceEpoch());
    const QString &revertID = input.revertID;
    for (int t = 0; t < turns.size(); ++t) {
        const Turn &turn = turns[t];
        const bool undone = !revertID.isEmpty() && turn.id >= revertID;
        if (undone && (t == 0 || turns[t - 1].id < revertID))
            rows << row(QStringLiteral("undo"), QStringLiteral("undo"));
        const bool live = input.busy && t == turns.size() - 1;
        if (turn.user >= 0) {
            const QString text = userText(entries[turn.user]);
            if (!text.isEmpty()) {
                rows << row(QStringLiteral("u:") + turn.id, QStringLiteral("user"), {{QStringLiteral("text"), text}},
                            undone);
            }
        }
        for (int s = 0; s < turn.steps.size(); ++s)
            addStep(rows, input, entries[turn.steps[s]], live && s == turn.steps.size() - 1, undone);

        const MessageEntry *last = turn.steps.isEmpty() ? nullptr : &entries[turn.steps.last()];
        bool stillWorking = false;
        if (live && last) {
            bool shows = false;
            for (const QJsonObject &part : last->parts) {
                const QString type = str(part, "type");
                shows = shows || type == QStringLiteral("text") || type == QStringLiteral("reasoning")
                    || type == QStringLiteral("tool");
            }
            stillWorking = num(obj(last->info, "time"), "completed") > 0 || !shows;
        }
        if ((live && turn.steps.isEmpty()) || stillWorking)
            rows << row(QStringLiteral("w:") + turn.id, QStringLiteral("working"), {}, undone);

        if (!turn.steps.isEmpty() || live) {
            const TurnStats stats = turnStats(entries, turn, now);
            static const QRegularExpression noThinking(QStringLiteral("no[-_ ]?think"),
                                                       QRegularExpression::CaseInsensitiveOption);
            const bool thinkingOff = !stats.variant.isEmpty() && noThinking.match(stats.variant).hasMatch();
            QString model = stats.modelID;
            if (!model.isEmpty()) {
                if (thinkingOff)
                    model += QStringLiteral(" · no thinking");
                else if (!stats.variant.isEmpty() && stats.variant != QStringLiteral("default"))
                    model += QStringLiteral(" · ") + stats.variant;
            }
            QVariantMap footer{
                {QStringLiteral("model"), model},
                {QStringLiteral("modelTitle"),
                 QStringLiteral("%1/%2%3").arg(stats.providerID, stats.modelID,
                                                stats.variant.isEmpty() ? QString() : QStringLiteral(" (%1)").arg(stats.variant))},
                {QStringLiteral("start"), stats.startMs},
                // a turn shown after it stopped keeps the time it had then
                {QStringLiteral("end"), stats.running && !live ? stats.startMs + stats.elapsedMs : stats.endMs},
                {QStringLiteral("live"), live},
                {QStringLiteral("tokens"), stats.tokens.input + stats.tokens.output > 0
                                               ? QStringLiteral("%1 in · %2 out").arg(formatTokens(stats.tokens.input), formatTokens(stats.tokens.output))
                                               : QString()},
                {QStringLiteral("tokensTitle"),
                 QStringLiteral("Input %1 · output %2 · reasoning %3 · cache read %4 · cache write %5")
                     .arg(stats.tokens.input).arg(stats.tokens.output).arg(stats.tokens.reasoning)
                     .arg(stats.tokens.cacheRead).arg(stats.tokens.cacheWrite)},
                {QStringLiteral("reasoning"), stats.tokens.reasoning > 0 ? formatTokens(stats.tokens.reasoning) : QString()},
                {QStringLiteral("toolCalls"), stats.toolCalls},
                {QStringLiteral("steps"), int(turn.steps.size())},
                {QStringLiteral("cost"), stats.cost > 0 ? formatCost(stats.cost) : QString()},
                {QStringLiteral("answer"), answerText(entries, turn)},
                {QStringLiteral("nextID"), t + 1 < turns.size() ? turns[t + 1].id : QString()},
                {QStringLiteral("userID"), turn.user >= 0 ? turn.id : QString()},
                {QStringLiteral("busy"), input.busy},
            };
            rows << row(QStringLiteral("f:") + turn.id, QStringLiteral("footer"), footer, undone);
        }
    }

    if (input.status.value(QStringLiteral("type")).toString() == QStringLiteral("retry")) {
        rows << row(QStringLiteral("retry"), QStringLiteral("error"),
                    {{QStringLiteral("text"), QStringLiteral("Retrying (attempt %1): %2")
                                                  .arg(input.status.value(QStringLiteral("attempt")).toInt())
                                                  .arg(str(input.status, "message"))}});
    }
    // The session error repeats the failed step's own error; show it only when no step carries it.
    bool lastStepFailed = false;
    if (!turns.isEmpty() && !turns.last().steps.isEmpty())
        lastStepFailed = hasError(entries[turns.last().steps.last()].info);
    if (!input.sessionError.isEmpty() && !input.busy && !lastStepFailed)
        rows << row(QStringLiteral("session-error"), QStringLiteral("error"), {{QStringLiteral("text"), input.sessionError}});
    if (turns.isEmpty() && !input.busy && rows.isEmpty())
        rows << row(QStringLiteral("empty"), QStringLiteral("empty"));
    return rows;
}

void ThreadModel::addStep(QList<Row> &rows, const Input &input, const MessageEntry &step, bool live, bool undone)
{
    const QString id = step.id();
    if (step.info.value(QStringLiteral("summary")).toBool()) {
        // a compaction's summary: a heading that opens onto every part of the step, as written
        const QString key = QStringLiteral("s:") + id;
        const bool expanded = input.expanded ? input.expanded->value(key, false) : false;
        rows << row(key, QStringLiteral("summary"), {{QStringLiteral("expanded"), expanded}}, undone);
        if (expanded) {
            for (const QJsonObject &part : step.parts)
                addPart(rows, input, part, live, undone);
        }
    } else {
        for (const QJsonObject &part : step.parts)
            addPart(rows, input, part, live, undone);
    }
    if (hasError(step.info)) {
        const QJsonValue error = step.info.value(QStringLiteral("error"));
        if (str(error.toObject(), "name") == QStringLiteral("MessageAbortedError"))
            rows << row(QStringLiteral("x:") + id, QStringLiteral("stopped"), {}, undone);
        else
            rows << row(QStringLiteral("e:") + id, QStringLiteral("error"), {{QStringLiteral("text"), errorText(error)}}, undone);
    }
}

void ThreadModel::addPart(QList<Row> &rows, const Input &input, const QJsonObject &part, bool live, bool undone)
{
    const QString id = str(part, "id");
    const QString type = str(part, "type");
    auto explicitly = [&](const QString &key, bool fallback) {
        return input.expanded && input.expanded->contains(key) ? input.expanded->value(key) : fallback;
    };

    if (type == QStringLiteral("reasoning")) {
        const QJsonObject time = obj(part, "time");
        const bool running = live && num(time, "end") <= 0;
        const QString text = str(part, "text").trimmed();
        if (text.isEmpty() && !running)
            return;
        rows << row(id, QStringLiteral("thinking"),
                    {{QStringLiteral("text"), text},
                     {QStringLiteral("running"), running},
                     {QStringLiteral("start"), num(time, "start")},
                     {QStringLiteral("end"), num(time, "end")},
                     {QStringLiteral("expanded"), explicitly(id, running)}},
                    undone);
    } else if (type == QStringLiteral("text")) {
        const QString text = str(part, "text");
        if (text.trimmed().isEmpty())
            return;
        const QList<ChatLogic::MarkdownBlock> blocks = ChatLogic::splitMarkdown(text);
        for (int i = 0; i < blocks.size(); ++i) {
            const QString key = id + QLatin1Char('#') + QString::number(i);
            if (blocks[i].code) {
                rows << row(key, QStringLiteral("code"),
                            {{QStringLiteral("text"), blocks[i].text}, {QStringLiteral("language"), blocks[i].language}},
                            undone);
            } else {
                rows << row(key, QStringLiteral("markdown"), {{QStringLiteral("text"), blocks[i].text}}, undone);
            }
        }
    } else if (type == QStringLiteral("compaction")) {
        rows << row(id, QStringLiteral("compaction"), {{QStringLiteral("auto"), part.value(QStringLiteral("auto")).toBool()}},
                    undone);
    } else if (type == QStringLiteral("tool")) {
        const QString tool = str(part, "tool");
        const QJsonObject state = obj(part, "state");
        const QString status = str(state, "status");
        const QJsonObject inputObject = obj(state, "input");
        const bool running = live && (status == QStringLiteral("pending") || status == QStringLiteral("running"));
        bool waiting = false;
        for (const QJsonObject &permission : input.permissions) {
            if (str(obj(permission, "tool"), "callID") == str(part, "callID")) {
                waiting = true;
                break;
            }
        }
        QString label = ChatLogic::humanizeTool(tool);
        QString summary;
        if (tool == QStringLiteral("task")) {
            const QString agent = str(inputObject, "subagent_type");
            label = QStringLiteral("Sub-agent") + (agent.isEmpty() ? QString() : QStringLiteral(" · ") + agent);
            summary = str(inputObject, "description");
        }
        const QString title = status == QStringLiteral("completed") || status == QStringLiteral("running") ? str(state, "title") : QString();
        if (summary.isEmpty())
            summary = status == QStringLiteral("error") ? str(state, "error") : (title != tool ? title : QString());
        const QString child = tool == QStringLiteral("task") ? ChatLogic::childSessionID(part) : QString();
        const QJsonObject time = obj(state, "time");
        const bool ended = status == QStringLiteral("completed") || status == QStringLiteral("error");

        ToolText &text = m_toolText[id];
        if (text.state != state || (text.input.isEmpty() && text.output.isEmpty())) {
            text.state = state;
            text.input = ChatLogic::pretty(state.value(QStringLiteral("input")));
            const QString output = status == QStringLiteral("completed") ? str(state, "output")
                : status == QStringLiteral("error")                      ? str(state, "error")
                                                                         : QString();
            text.output = output.size() > outputLimit ? ChatLogic::pretty(output.left(outputLimit)) + QStringLiteral("\n…")
                                                      : ChatLogic::pretty(output);
        }
        rows << row(id, QStringLiteral("tool"),
                    {{QStringLiteral("tool"), tool},
                     {QStringLiteral("label"), label},
                     {QStringLiteral("summary"), waiting ? QStringLiteral("Waiting for your approval") : summary.section(QLatin1Char('\n'), 0, 0)},
                     {QStringLiteral("status"), status},
                     {QStringLiteral("running"), running},
                     {QStringLiteral("waiting"), waiting},
                     {QStringLiteral("start"), num(time, "start")},
                     {QStringLiteral("end"), ended ? num(time, "end") : 0.0},
                     {QStringLiteral("input"), text.input},
                     {QStringLiteral("output"), text.output},
                     {QStringLiteral("outputLabel"), status == QStringLiteral("error") ? QStringLiteral("Error") : QStringLiteral("Output")},
                     {QStringLiteral("child"), child},
                     {QStringLiteral("expanded"), explicitly(id, waiting || (!child.isEmpty() && running))}},
                    undone);
    } else if (type == QStringLiteral("patch")) {
        QStringList names;
        const QJsonArray files = part.value(QStringLiteral("files")).toArray();
        for (const QJsonValue &file : files)
            names << file.toString().section(QLatin1Char('/'), -1);
        rows << row(id, QStringLiteral("patch"), {{QStringLiteral("count"), int(files.size())}, {QStringLiteral("names"), names.join(QStringLiteral(", "))}},
                    undone);
    } else if (type == QStringLiteral("file")) {
        const QString name = str(part, "filename");
        rows << row(id, QStringLiteral("file"), {{QStringLiteral("name"), name.isEmpty() ? str(part, "url") : name}}, undone);
    } else if (type == QStringLiteral("retry")) {
        rows << row(id, QStringLiteral("error"),
                    {{QStringLiteral("text"), QStringLiteral("Retrying: ") + errorText(part.value(QStringLiteral("error")))}}, undone);
    }
}
