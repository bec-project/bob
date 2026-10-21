#include "chatlogic.h"

#include "turns.h"

#include <QDir>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLocale>
#include <QRegularExpression>
#include <QUrl>

#include <cmath>

namespace ChatLogic {

namespace {

QString str(const QJsonObject &object, const char *key)
{
    return object.value(QLatin1String(key)).toString();
}

QString partsText(const QList<QJsonObject> &parts, const QString &type)
{
    QStringList texts;
    for (const QJsonObject &part : parts) {
        if (str(part, "type") != type || part.value(QStringLiteral("synthetic")).toBool())
            continue;
        const QString text = str(part, "text").trimmed();
        if (!text.isEmpty())
            texts << text;
    }
    return texts.join(QStringLiteral("\n\n"));
}

QString compactJson(const QJsonValue &value)
{
    if (value.isObject())
        return QString::fromUtf8(QJsonDocument(value.toObject()).toJson(QJsonDocument::Compact));
    if (value.isArray())
        return QString::fromUtf8(QJsonDocument(value.toArray()).toJson(QJsonDocument::Compact));
    if (value.isUndefined())
        return {};
    return QString::fromUtf8(QJsonDocument(QJsonArray{value}).toJson(QJsonDocument::Compact)).mid(1).chopped(1);
}

void writePretty(const QJsonValue &value, int depth, QString &out)
{
    const QString indent(2 * (depth + 1), QLatin1Char(' '));
    const QString close(2 * depth, QLatin1Char(' '));
    if (value.isObject()) {
        const QJsonObject object = value.toObject();
        if (object.isEmpty()) {
            out += QStringLiteral("{}");
            return;
        }
        out += QStringLiteral("{\n");
        int i = 0;
        for (auto it = object.begin(); it != object.end(); ++it, ++i) {
            out += indent + compactJson(QJsonValue(it.key())) + QStringLiteral(": ");
            writePretty(it.value(), depth + 1, out);
            out += i + 1 < object.size() ? QStringLiteral(",\n") : QStringLiteral("\n");
        }
        out += close + QLatin1Char('}');
    } else if (value.isArray()) {
        const QJsonArray array = value.toArray();
        if (array.isEmpty()) {
            out += QStringLiteral("[]");
            return;
        }
        out += QStringLiteral("[\n");
        for (int i = 0; i < array.size(); ++i) {
            out += indent;
            writePretty(array[i], depth + 1, out);
            out += i + 1 < array.size() ? QStringLiteral(",\n") : QStringLiteral("\n");
        }
        out += close + QLatin1Char(']');
    } else {
        out += compactJson(value);
    }
}

QString toolLine(const QJsonObject &part)
{
    const QJsonObject state = part.value(QStringLiteral("state")).toObject();
    const QString title = str(state, "title");
    const QString status = str(state, "status") == QStringLiteral("error")
        ? QStringLiteral(" (failed: %1)").arg(str(state, "error").section(QLatin1Char('\n'), 0, 0))
        : QString();
    QString input = compactJson(state.value(QStringLiteral("input")));
    if (input.size() > 200)
        input = input.left(200) + QStringLiteral("…");
    QString line = QStringLiteral("- `%1`").arg(str(part, "tool"));
    if (!title.isEmpty())
        line += QLatin1Char(' ') + title;
    line += status;
    if (!input.isEmpty() && input != QStringLiteral("{}"))
        line += QStringLiteral(" `%1`").arg(input);
    return line;
}

}  // namespace

QStringList promptHistory(const QList<MessageEntry> &entries, const QStringList &sent)
{
    QStringList own;
    for (const MessageEntry &entry : entries) {
        if (entry.role() != QStringLiteral("user"))
            continue;
        const QString text = promptText(entry).trimmed();
        if (!text.isEmpty())
            own << text;
    }
    const QStringList &list = own.isEmpty() ? sent : own;
    QStringList history;
    for (const QString &text : list) {
        if (history.isEmpty() || history.last() != text)
            history << text;
    }
    return history;
}

std::optional<RecallStep> recallStep(const QStringList &history, const Recall &recall, const QString &current,
                                     bool up)
{
    if (up) {
        if (recall.index < 0) {
            if (history.isEmpty())
                return std::nullopt;
            const int index = int(history.size()) - 1;
            return RecallStep{{index, current}, history[index]};
        }
        if (recall.index == 0)
            return std::nullopt;
        return RecallStep{{recall.index - 1, recall.draft}, history[recall.index - 1]};
    }
    if (recall.index < 0)
        return std::nullopt;
    if (recall.index >= history.size() - 1)
        return RecallStep{{}, recall.draft};
    return RecallStep{{recall.index + 1, recall.draft}, history[recall.index + 1]};
}

QString chatMarkdown(const QJsonObject &session, const QList<MessageEntry> &entries, bool thinking,
                     const QDateTime &exported, double now)
{
    const QString title = str(session, "title");
    QStringList lines = {QStringLiteral("# %1").arg(title.isEmpty() ? QStringLiteral("Chat") : title), QString()};
    if (!session.isEmpty()) {
        const double created = session.value(QStringLiteral("time")).toObject().value(QStringLiteral("created")).toDouble();
        const QString format = QStringLiteral("yyyy-MM-dd HH:mm");
        lines << QStringLiteral("Exported %1 · started %2")
                     .arg(exported.toUTC().toString(format),
                          QDateTime::fromMSecsSinceEpoch(qint64(created), QTimeZone::UTC).toString(format))
              << QString();
    }
    for (const Turn &turn : groupTurns(entries)) {
        if (turn.user >= 0)
            lines << QStringLiteral("## You") << QString() << partsText(entries[turn.user].parts, QStringLiteral("text")) << QString();
        if (turn.steps.isEmpty())
            continue;
        QStringList tools;
        QStringList answers;
        QStringList thoughts;
        for (int index : turn.steps) {
            const QList<QJsonObject> &parts = entries[index].parts;
            for (const QJsonObject &part : parts) {
                if (str(part, "type") == QStringLiteral("tool"))
                    tools << toolLine(part);
                if (thinking && str(part, "type") == QStringLiteral("reasoning") && !str(part, "text").trimmed().isEmpty())
                    thoughts << str(part, "text").trimmed();
            }
            const QString answer = partsText(parts, QStringLiteral("text"));
            if (!answer.isEmpty())
                answers << answer;
        }
        lines << QStringLiteral("## Assistant") << QString();
        if (!thoughts.isEmpty())
            lines << QStringLiteral("<details><summary>Thinking</summary>") << QString() << thoughts << QString()
                  << QStringLiteral("</details>") << QString();
        if (!tools.isEmpty())
            lines << QStringLiteral("Tools:") << QString() << tools << QString();
        if (!answers.isEmpty())
            lines << answers.join(QStringLiteral("\n\n")) << QString();
        const TurnStats stats = turnStats(entries, turn, now);
        QStringList usage;
        if (!stats.modelID.isEmpty())
            usage << stats.modelID;
        usage << formatDuration(stats.elapsedMs) << QStringLiteral("%1 in").arg(formatTokens(stats.tokens.input))
              << QStringLiteral("%1 out").arg(formatTokens(stats.tokens.output));
        lines << QStringLiteral("*%1*").arg(usage.join(QStringLiteral(" · "))) << QString();
    }
    QString text = lines.join(QLatin1Char('\n'));
    while (text.endsWith(QLatin1Char('\n')) || text.endsWith(QLatin1Char(' ')))
        text.chop(1);
    return text + QLatin1Char('\n');
}

QString exportFileName(const QJsonObject &session)
{
    QString title = str(session, "title");
    if (title.isEmpty())
        title = QStringLiteral("chat");
    static const QRegularExpression unsafe(QStringLiteral("[^\\p{L}\\p{N}\\- ]+"));
    static const QRegularExpression spaces(QStringLiteral("\\s+"));
    QString base = title.remove(unsafe).trimmed().replace(spaces, QStringLiteral("-")).left(60);
    return (base.isEmpty() ? QStringLiteral("chat") : base) + QStringLiteral(".md");
}

PermissionView describePermission(const QJsonObject &request, const QJsonObject &tool)
{
    const QJsonObject meta = request.value(QStringLiteral("metadata")).toObject();
    QStringList patterns;
    for (const QJsonValue &pattern : request.value(QStringLiteral("patterns")).toArray()) {
        if (pattern.toString() != QStringLiteral("*"))
            patterns << pattern.toString();
    }
    const QString joined = patterns.join(QLatin1Char('\n'));
    auto metaText = [&](const char *key) {
        const QJsonValue value = meta.value(QLatin1String(key));
        if (value.isUndefined() || value.isNull())
            return joined;
        return value.isString() ? value.toString() : compactJson(value);
    };
    const QString input = tool.isEmpty()
        ? QString()
        : prettyJson(tool.value(QStringLiteral("state")).toObject().value(QStringLiteral("input")));
    const QString permission = str(request, "permission");
    if (permission == QStringLiteral("bash"))
        return {QStringLiteral("Run this command?"), metaText("command")};
    if (permission == QStringLiteral("edit")) {
        QString detail = metaText("filepath");
        const QString diff = str(meta, "diff");
        if (!diff.isEmpty())
            detail += QStringLiteral("\n\n") + diff;
        return {QStringLiteral("Change this file?"), detail};
    }
    if (permission == QStringLiteral("external_directory"))
        return {QStringLiteral("Work outside the workspace?"), joined};
    if (permission == QStringLiteral("webfetch"))
        return {QStringLiteral("Fetch this page?"), metaText("url")};
    if (permission == QStringLiteral("doom_loop"))
        return {QStringLiteral("The assistant repeats the same call. Let it continue?"), input};
    QStringList detail;
    if (!joined.isEmpty())
        detail << joined;
    if (!input.isEmpty())
        detail << input;
    return {QStringLiteral("Allow %1?").arg(permission), detail.join(QStringLiteral("\n\n"))};
}

std::optional<QJsonObject> toConnection(const QJsonValue &value)
{
    if (!value.isObject())
        return std::nullopt;
    const QJsonObject object = value.toObject();
    if (!object.value(QStringLiteral("url")).isString() || !object.value(QStringLiteral("password")).isString())
        return std::nullopt;
    return object;
}

std::optional<QJsonObject> parseConnection(const QString &text, const QString &password)
{
    const QString trimmed = text.trimmed();
    if (trimmed.isEmpty())
        return std::nullopt;
    if (trimmed.startsWith(QLatin1Char('{'))) {
        const QJsonDocument document = QJsonDocument::fromJson(trimmed.toUtf8());
        return document.isObject() ? toConnection(document.object()) : std::nullopt;
    }
    const QUrl url(trimmed, QUrl::StrictMode);
    if (!url.isValid() || (url.scheme() != QStringLiteral("http") && url.scheme() != QStringLiteral("https"))
        || url.host().isEmpty()) {
        return std::nullopt;
    }
    QString path = url.path();
    while (path.endsWith(QLatin1Char('/')))
        path.chop(1);
    const QString origin = url.toString(QUrl::RemovePath | QUrl::RemoveQuery | QUrl::RemoveFragment
                                        | QUrl::RemoveUserInfo | QUrl::StripTrailingSlash);
    return QJsonObject{{QStringLiteral("url"), origin + path}, {QStringLiteral("password"), password}};
}

QList<MarkdownBlock> splitMarkdown(const QString &text)
{
    static const QRegularExpression opening(QStringLiteral("^( {0,3})(`{3,}|~{3,})\\s*([^\\s`]*)[^`]*$"));
    QList<MarkdownBlock> blocks;
    QStringList prose;
    auto flushProse = [&] {
        while (!prose.isEmpty() && prose.first().trimmed().isEmpty())
            prose.removeFirst();
        while (!prose.isEmpty() && prose.last().trimmed().isEmpty())
            prose.removeLast();
        if (!prose.isEmpty())
            blocks.append({false, prose.join(QLatin1Char('\n')), {}});
        prose.clear();
    };
    const QStringList lines = text.split(QLatin1Char('\n'));
    for (int i = 0; i < lines.size(); ++i) {
        const QRegularExpressionMatch open = opening.match(lines[i]);
        if (!open.hasMatch()) {
            prose << lines[i];
            continue;
        }
        flushProse();
        const int indent = int(open.captured(1).size());
        const QString fence = open.captured(2);
        const QRegularExpression closing(QStringLiteral("^ {0,3}%1{%2,}\\s*$")
                                             .arg(QRegularExpression::escape(fence.left(1)))
                                             .arg(fence.size()));
        QStringList code;
        int j = i + 1;
        for (; j < lines.size(); ++j) {
            if (closing.match(lines[j]).hasMatch())
                break;
            QString line = lines[j];
            int strip = 0;
            while (strip < indent && strip < line.size() && line[strip] == QLatin1Char(' '))
                ++strip;
            code << line.mid(strip);
        }
        blocks.append({true, code.join(QLatin1Char('\n')), open.captured(3)});
        i = j;
    }
    flushProse();
    return blocks;
}

QString prettyJson(const QJsonValue &value)
{
    QString out;
    writePretty(value, 0, out);
    return out;
}

QString pretty(const QJsonValue &value)
{
    if (value.isString()) {
        const QString text = value.toString();
        const QString trimmed = text.trimmed();
        if (trimmed.startsWith(QLatin1Char('{')) || trimmed.startsWith(QLatin1Char('['))) {
            QJsonParseError error;
            const QJsonDocument document = QJsonDocument::fromJson(trimmed.toUtf8(), &error);
            if (error.error == QJsonParseError::NoError)
                return prettyJson(document.isObject() ? QJsonValue(document.object()) : QJsonValue(document.array()));
        }
        return text;
    }
    return prettyJson(value);
}

QString humanizeTool(const QString &tool)
{
    static const QRegularExpression separators(QStringLiteral("[_-]+"));
    QString text = QString(tool).replace(separators, QStringLiteral(" "));
    if (!text.isEmpty())
        text[0] = text[0].toUpper();
    return text;
}

QString childSessionID(const QJsonObject &toolPart)
{
    const QJsonObject meta = toolPart.value(QStringLiteral("state")).toObject().value(QStringLiteral("metadata")).toObject();
    for (const QString &key : {QStringLiteral("sessionId"), QStringLiteral("sessionID")}) {
        if (meta.value(key).isString())
            return meta.value(key).toString();
    }
    const QJsonValue own = toolPart.value(QStringLiteral("metadata")).toObject().value(QStringLiteral("sessionId"));
    return own.isString() ? own.toString() : QString();
}

QString noThinkingVariant(const QJsonObject &model)
{
    static const QRegularExpression pattern(QStringLiteral("no[-_ ]?think"), QRegularExpression::CaseInsensitiveOption);
    for (const QString &name : model.value(QStringLiteral("variants")).toObject().keys()) {
        if (pattern.match(name).hasMatch())
            return name;
    }
    return {};
}

QString relativeTime(double ms, double now)
{
    const double seconds = qMax(0.0, (now - ms) / 1000);
    if (seconds < 60)
        return QStringLiteral("now");
    if (seconds < 3600)
        return QStringLiteral("%1m").arg(qint64(seconds / 60));
    if (seconds < 86400)
        return QStringLiteral("%1h").arg(qint64(seconds / 3600));
    if (seconds < 7 * 86400)
        return QStringLiteral("%1d").arg(qint64(seconds / 86400));
    return QLocale().toString(QDateTime::fromMSecsSinceEpoch(qint64(ms)).date(), QStringLiteral("MMM d"));
}

QString dayGroup(double ms, const QDateTime &now)
{
    const double start = QDateTime(now.date(), QTime(0, 0), now.timeZone()).toMSecsSinceEpoch();
    if (ms >= start)
        return QStringLiteral("Today");
    if (ms >= start - 86400000.0)
        return QStringLiteral("Yesterday");
    if (ms >= start - 6 * 86400000.0)
        return QStringLiteral("This week");
    return QLocale().toString(QDateTime::fromMSecsSinceEpoch(qint64(ms)).date(), QStringLiteral("MMMM yyyy"));
}

int clampWidth(double width, int min, int windowWidth)
{
    return int(std::round(qMin(qMax(width, double(min)), double(qMax(min, windowWidth - 480)))));
}

QString freePath(const QString &dir, const QString &name)
{
    const QFileInfo file(name);
    QString stem = file.completeBaseName();
    QString extension = file.suffix();
    if (stem.isEmpty())
        stem = QStringLiteral("chat");
    if (extension.isEmpty())
        extension = QStringLiteral("md");
    QString candidate = QDir(dir).filePath(QStringLiteral("%1.%2").arg(stem, extension));
    for (int n = 2; QFileInfo::exists(candidate); ++n)
        candidate = QDir(dir).filePath(QStringLiteral("%1 (%2).%3").arg(stem).arg(n).arg(extension));
    return candidate;
}

QString patchHtml(const QString &patch, const QString &addColor, const QString &deleteColor, const QString &hunkColor)
{
    static const QRegularExpression header(QStringLiteral("^(diff --git|index |--- |\\+\\+\\+ |=+$|Index: )"));
    QStringList html;
    for (const QString &line : patch.split(QLatin1Char('\n'))) {
        if (header.match(line).hasMatch())
            continue;
        const QString escaped = line.isEmpty() ? QStringLiteral(" ") : line.toHtmlEscaped();
        QString color;
        if (line.startsWith(QStringLiteral("@@")))
            color = hunkColor;
        else if (line.startsWith(QLatin1Char('+')))
            color = addColor;
        else if (line.startsWith(QLatin1Char('-')))
            color = deleteColor;
        html << (color.isEmpty() ? escaped : QStringLiteral("<span style=\"color:%1\">%2</span>").arg(color, escaped));
    }
    return QStringLiteral("<pre style=\"margin:0\">%1</pre>").arg(html.join(QLatin1Char('\n')));
}

}  // namespace ChatLogic
