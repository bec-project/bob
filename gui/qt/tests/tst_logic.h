#pragma once

#include <QObject>

// The chat's rules: the event reducer, turns, history, export, parsing and the thread's rows.
// (The cases live in tst_logic.cpp, which moc does not read: it trips over raw strings in macros.)
class TestLogic : public QObject
{
    Q_OBJECT

private slots:
    void readsTheConnectionBobPrints();
    void splitsTheEventStream();
    void streamsTextDeltasIntoThePart();
    void ignoresEventsItDoesNotUse();
    void keepsMessagesInIdOrder();
    void tracksRequestsUntilAnswered();
    void dropsADeletedSession();
    void recordsSessionErrorsInWords();
    void groupsTheStepsOfEachPrompt();
    void formatsNumbers();
    void takesTheChatsOwnPrompts();
    void stepsThroughEarlierPrompts();
    void exportsTheChatAsMarkdown();
    void numbersADownloadThatExists();
    void keepsADraggedWidthInTheWindow();
    void splitsCodeBlocksOutOfMarkdown();
    void describesPermissionRequests();
    void makesToolTextReadable();
    void buildsTheThreadsRows();
    void opensACompactionSummaryOntoAllItsParts();
    void showsAnEmptyChatAndASubAgentsSteps();
    void readsAndSavesSettings();
};
