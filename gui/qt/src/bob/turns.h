#pragma once

#include "sessiondata.h"

struct Tokens
{
    double input = 0;
    double output = 0;
    double reasoning = 0;
    double cacheRead = 0;
    double cacheWrite = 0;

    Tokens &operator+=(const Tokens &other);
};

Tokens messageTokens(const QJsonObject &info);

// One prompt and every assistant message (model step) it produced, as indexes into the session's
// message list.
struct Turn
{
    QString id;
    int user = -1;
    QList<int> steps;
};

struct TurnStats
{
    // Milliseconds from the prompt to the last completed step, or until now while running
    double elapsedMs = 0;
    bool running = false;
    // When the turn began, and when its last step completed (0 while running)
    double startMs = 0;
    double endMs = 0;
    Tokens tokens;
    double cost = 0;
    // The context the last step sent: its input plus cached input
    double contextTokens = 0;
    QString modelID;
    QString providerID;
    QString variant;
    int toolCalls = 0;
};

// Groups messages into turns: each user message with the assistant messages answering it.
QList<Turn> groupTurns(const QList<MessageEntry> &entries);

TurnStats turnStats(const QList<MessageEntry> &entries, const Turn &turn, double now);

QString formatTokens(double value);
QString formatDuration(double ms);
QString formatCost(double cost);
