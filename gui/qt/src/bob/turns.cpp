#include "turns.h"

#include <QHash>
#include <cmath>

namespace {

double number(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toDouble();
}

QJsonObject object(const QJsonObject &parent, const char *key)
{
    return parent.value(QLatin1String(key)).toObject();
}

QString fixed(double value, int decimals)
{
    return QString::number(value, 'f', decimals);
}

}  // namespace

Tokens &Tokens::operator+=(const Tokens &other)
{
    input += other.input;
    output += other.output;
    reasoning += other.reasoning;
    cacheRead += other.cacheRead;
    cacheWrite += other.cacheWrite;
    return *this;
}

Tokens messageTokens(const QJsonObject &info)
{
    const QJsonObject tokens = object(info, "tokens");
    const QJsonObject cache = object(tokens, "cache");
    return {number(tokens, "input"), number(tokens, "output"), number(tokens, "reasoning"), number(cache, "read"),
            number(cache, "write")};
}

QList<Turn> groupTurns(const QList<MessageEntry> &entries)
{
    QList<Turn> turns;
    QHash<QString, int> byUser;
    for (int i = 0; i < entries.size(); ++i) {
        const MessageEntry &entry = entries[i];
        const QString role = entry.role();
        if (role == QStringLiteral("user")) {
            byUser.insert(entry.id(), int(turns.size()));
            turns.append({entry.id(), i, {}});
        } else if (role == QStringLiteral("assistant")) {
            const QString parent = entry.info.value(QStringLiteral("parentID")).toString();
            const int owner = byUser.value(parent, int(turns.size()) - 1);
            if (owner >= 0)
                turns[owner].steps.append(i);
            else
                turns.append({entry.id(), -1, {i}});
        }
    }
    return turns;
}

TurnStats turnStats(const QList<MessageEntry> &entries, const Turn &turn, double now)
{
    TurnStats stats;
    double end = 0;
    for (int index : turn.steps) {
        const MessageEntry &step = entries[index];
        stats.tokens += messageTokens(step.info);
        stats.cost += number(step.info, "cost");
        for (const QJsonObject &part : step.parts) {
            if (part.value(QStringLiteral("type")).toString() == QStringLiteral("tool"))
                ++stats.toolCalls;
        }
        const double completed = number(object(step.info, "time"), "completed");
        if (completed > 0)
            end = qMax(end, completed);
        else if (step.info.value(QStringLiteral("error")).isUndefined() || step.info.value(QStringLiteral("error")).isNull())
            stats.running = true;
    }
    const QJsonObject *last = turn.steps.isEmpty() ? nullptr : &entries[turn.steps.last()].info;
    const QJsonObject *user = turn.user >= 0 ? &entries[turn.user].info : nullptr;
    double start = now;
    if (user)
        start = number(object(*user, "time"), "created");
    else if (!turn.steps.isEmpty())
        start = number(object(entries[turn.steps.first()].info, "time"), "created");
    // A step reports its tokens when it finishes, so the context is the last step that has some.
    for (auto it = turn.steps.crbegin(); it != turn.steps.crend(); ++it) {
        const Tokens tokens = messageTokens(entries[*it].info);
        if (tokens.input > 0) {
            stats.contextTokens = tokens.input + tokens.cacheRead;
            break;
        }
    }
    // a stopped step may have no completion time: end the turn where its last step began
    const double lastCreated = last ? number(object(*last, "time"), "created") : start;
    const double finish = stats.running ? now : (end > 0 ? end : lastCreated);
    stats.startMs = start;
    stats.endMs = stats.running ? 0 : finish;
    stats.elapsedMs = qMax(0.0, finish - start);
    const QJsonObject userModel = user ? object(*user, "model") : QJsonObject();
    auto pick = [&](const char *stepKey, const char *userKey) {
        const QString fromStep = last ? last->value(QLatin1String(stepKey)).toString() : QString();
        return fromStep.isEmpty() ? userModel.value(QLatin1String(userKey)).toString() : fromStep;
    };
    stats.modelID = pick("modelID", "modelID");
    stats.providerID = pick("providerID", "providerID");
    stats.variant = pick("variant", "variant");
    return stats;
}

QString formatTokens(double value)
{
    if (value < 1000)
        return QString::number(qint64(value));
    if (value < 10000)
        return fixed(value / 1000, 1) + QLatin1Char('k');
    if (value < 1000000)
        return QString::number(qint64(std::round(value / 1000))) + QLatin1Char('k');
    return fixed(value / 1000000, 1) + QLatin1Char('M');
}

QString formatDuration(double ms)
{
    const double seconds = ms / 1000;
    if (seconds < 1)
        return QStringLiteral("%1 ms").arg(qint64(std::round(ms)));
    if (seconds < 60)
        return QStringLiteral("%1 s").arg(fixed(seconds, 1));
    const qint64 minutes = qint64(std::floor(seconds / 60));
    if (minutes < 60)
        return QStringLiteral("%1 min %2 s").arg(minutes).arg(qint64(std::round(std::fmod(seconds, 60))));
    return QStringLiteral("%1 h %2 min").arg(minutes / 60).arg(minutes % 60);
}

QString formatCost(double cost)
{
    return cost >= 0.01 ? QStringLiteral("$%1").arg(fixed(cost, 2)) : QStringLiteral("$%1").arg(fixed(cost, 4));
}
