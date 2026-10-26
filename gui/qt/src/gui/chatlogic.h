#pragma once

#include "sessiondata.h"

#include <QDateTime>
#include <QJsonObject>
#include <QStringList>

#include <optional>

// The chat's rules that do not depend on Qt Quick, kept apart so they can be tested.
namespace ChatLogic {

// The prompts Up steps through, oldest first: the open chat's own, or, in a chat that has none
// yet, the ones sent from this window. Repeats in a row count once.
QStringList promptHistory(const QList<MessageEntry> &entries, const QStringList &sent);

// Where the composer stands in the history while the user steps through it.
struct Recall
{
    // -1 while not stepping through the history
    int index = -1;
    // What the composer held before the first step, given back after the newest prompt
    QString draft;
};

struct RecallStep
{
    Recall recall;
    QString text;
};

// One press of Up (`up`) or Down: the next position and the text to show, or nothing when the key
// should move the caret as usual (no older prompt, or not stepping through the history).
std::optional<RecallStep> recallStep(const QStringList &history, const Recall &recall, const QString &current,
                                     bool up);

// The chat as Markdown: prompts, answers, the tools each answer used, and its usage.
QString chatMarkdown(const QJsonObject &session, const QList<MessageEntry> &entries, bool thinking,
                     const QDateTime &exported, double now);

// A file name for the export: the chat title, made safe.
QString exportFileName(const QJsonObject &session);

struct PermissionView
{
    QString title;
    QString detail;
};

// What a permission request asks, in words; `tool` is the tool call it is about, if known.
PermissionView describePermission(const QJsonObject &request, const QJsonObject &tool);

// Model output split into Markdown prose and fenced code blocks, so each code block gets its own
// view (with a copy button). An unclosed fence runs to the end, as while an answer streams.
struct MarkdownBlock
{
    bool code = false;
    QString text;
    QString language;
};
QList<MarkdownBlock> splitMarkdown(const QString &text);

// JSON indented by two spaces, like JSON.stringify(value, null, 2).
QString prettyJson(const QJsonValue &value);
// A tool's input or output for reading: JSON (also JSON in a string) indented, text as it is.
QString pretty(const QJsonValue &value);
// "bec_device_read" -> "Bec device read"
QString humanizeTool(const QString &tool);
// The sub-agent session a task tool call started, if OpenCode reported it.
QString childSessionID(const QJsonObject &toolPart);

// The variant that switches thinking off, when the model's configuration defines one.
QString noThinkingVariant(const QJsonObject &model);

// "now", "5m", "3h", "2d" or a date, for the chat list.
QString relativeTime(double ms, double now);
// Today, Yesterday, This week, or the month, for grouping the chat list.
QString dayGroup(double ms, const QDateTime &now);

// Keeps a resized width inside the window: at least min, and leaving room for the chat.
int clampWidth(double width, int min, int windowWidth);

// A free path for `name` in `dir`: the name itself, or the name with " (2)", " (3)", … added.
QString freePath(const QString &dir, const QString &name);

// A unified diff as HTML, added and removed lines coloured; file headers are left out.
QString patchHtml(const QString &patch, const QString &addColor, const QString &deleteColor,
                  const QString &hunkColor);

}  // namespace ChatLogic
