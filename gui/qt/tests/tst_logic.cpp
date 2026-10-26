#include "tst_logic.h"

#include "bobsettings.h"
#include "chatlogic.h"
#include "connection.h"
#include "eventstream.h"
#include "runningbobs.h"
#include "sessiondata.h"
#include "threadmodel.h"
#include "turns.h"

#include <QJsonDocument>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTest>

namespace {

QJsonObject json(const char *text)
{
    return QJsonDocument::fromJson(text).object();
}

QJsonObject serverEvent(const QString &type, const QJsonObject &properties)
{
    return {{"type", type}, {"properties", properties}};
}

MessageEntry user(const QString &id, double created, const QString &text = QStringLiteral("hi"))
{
    return {QJsonObject{{"id", id}, {"sessionID", "ses_1"}, {"role", "user"}, {"time", QJsonObject{{"created", created}}},
                        {"model", QJsonObject{{"providerID", "p"}, {"modelID", "m"}}}},
            {QJsonObject{{"id", "prt_" + id}, {"sessionID", "ses_1"}, {"messageID", id}, {"type", "text"}, {"text", text}}}};
}

MessageEntry assistant(const QString &id, const QString &parent, double created, double completed = 0, double input = 10)
{
    QJsonObject time{{"created", created}};
    if (completed > 0)
        time.insert("completed", completed);
    return {QJsonObject{{"id", id},
                        {"sessionID", "ses_1"},
                        {"role", "assistant"},
                        {"parentID", parent},
                        {"time", time},
                        {"modelID", "m"},
                        {"providerID", "p"},
                        {"cost", 0},
                        {"tokens", QJsonObject{{"input", input}, {"output", 2}, {"reasoning", 1},
                                                {"cache", QJsonObject{{"read", 0}, {"write", 0}}}}}},
            {}};
}

QStringList keys(const ThreadModel &model)
{
    QStringList list;
    for (const auto &row : model.rows())
        list << row.key;
    return list;
}

QStringList kinds(const ThreadModel &model)
{
    QStringList list;
    for (const auto &row : model.rows())
        list << row.kind;
    return list;
}

}  // namespace

// ---------- the connection and the event stream ----------

void TestLogic::readsTheConnectionBobPrints()
{
    const auto connection = Connection::parse(
        R"({"url":"http://127.0.0.1:52997","password":"s3cret","container":"bob-1","readOnly":false})", QString());
    QVERIFY(connection);
    QCOMPARE(connection->value("url").toString(), QStringLiteral("http://127.0.0.1:52997"));
    QCOMPARE(connection->value("container").toString(), QStringLiteral("bob-1"));

    const auto bare = Connection::parse(QStringLiteral("http://host:4096/"), QStringLiteral("pw"));
    QVERIFY(bare);
    QCOMPARE(*bare, (QJsonObject{{"url", "http://host:4096"}, {"password", "pw"}}));

    QVERIFY(!Connection::parse(QString(), QString()));
    QVERIFY(!Connection::parse(QStringLiteral(R"({"url":"http://x"})"), QString()));
    QVERIFY(!Connection::parse(QStringLiteral("ftp://x"), QString()));
    QVERIFY(!Connection::parse(QStringLiteral("{broken"), QString()));

    QVERIFY(Connection::fromJson(json(R"({"url":"http://127.0.0.1:1","password":"p","container":"c"})")));
    QVERIFY(!Connection::fromJson(json(R"({"url":"http://127.0.0.1:1"})")));
    QVERIFY(!Connection::fromJson(QJsonValue("text")));
}

void TestLogic::splitsTheEventStream()
{
    SseParser parser;
    QCOMPARE(parser.push(R"(data: {"a":)"), QStringList());
    QCOMPARE(parser.push("1}\n\ndata: {\"b\":2}\r\n"), QStringList{R"({"a":1})"});
    QCOMPARE(parser.push("\r\n: comment\n\n"), QStringList{R"({"b":2})"});
    QCOMPARE(parser.push("data: one\ndata: two\n\n"), QStringList{"one\ntwo"});
}

// ---------- the event reducer ----------

void TestLogic::streamsTextDeltasIntoThePart()
{
    SessionData data;
    data.messages.insert("ses_1", {user("msg_1", 1), assistant("msg_2", "msg_1", 2)});
    applyEvent(data, serverEvent("message.part.updated",
                           {{"part", json(R"({"id":"prt_a","sessionID":"ses_1","messageID":"msg_2","type":"text","text":""})")}}));
    applyEvent(data, serverEvent("message.part.delta", json(R"({"sessionID":"ses_1","messageID":"msg_2","partID":"prt_a","field":"text","delta":"Hel"})")));
    const EventEffect effect = applyEvent(
        data, serverEvent("message.part.delta", json(R"({"sessionID":"ses_1","messageID":"msg_2","partID":"prt_a","field":"text","delta":"lo"})")));
    QCOMPARE(data.messages["ses_1"][1].parts[0].value("text").toString(), QStringLiteral("Hello"));
    QCOMPARE(effect.areas, int(EventEffect::Messages));
    QCOMPARE(effect.sessionID, QStringLiteral("ses_1"));
}

void TestLogic::ignoresEventsItDoesNotUse()
{
    SessionData data;
    data.messages.insert("ses_1", {});
    QVERIFY(!applyEvent(data, serverEvent("server.heartbeat", {})).changed());
    QVERIFY(!applyEvent(data, serverEvent("message.part.delta", json(R"({"sessionID":"ses_x","messageID":"m","partID":"p","field":"text","delta":"x"})"))).changed());
    // a part before its message waits for message.updated
    QVERIFY(!applyEvent(data, serverEvent("message.part.updated", {{"part", json(R"({"id":"p","sessionID":"ses_1","messageID":"msg_9","type":"text"})")}})).changed());
}

void TestLogic::keepsMessagesInIdOrder()
{
    SessionData data;
    data.messages.insert("ses_1", {user("msg_1", 1)});
    applyEvent(data, serverEvent("message.updated", {{"info", assistant("msg_3", "msg_1", 3).info}}));
    applyEvent(data, serverEvent("message.updated", {{"info", assistant("msg_2", "msg_1", 2).info}}));
    applyEvent(data, serverEvent("message.updated", {{"info", assistant("msg_2", "msg_1", 2, 5).info}}));
    QStringList ids;
    for (const MessageEntry &entry : data.messages["ses_1"])
        ids << entry.id();
    QCOMPARE(ids, (QStringList{"msg_1", "msg_2", "msg_3"}));
    QCOMPARE(data.messages["ses_1"][1].info["time"].toObject()["completed"].toDouble(), 5.0);
}

void TestLogic::tracksRequestsUntilAnswered()
{
    SessionData data;
    applyEvent(data, serverEvent("permission.asked", json(R"({"id":"per_1","sessionID":"ses_1","permission":"bash","patterns":["ls"],"metadata":{},"always":[]})")));
    QCOMPARE(data.permissions.keys(), QStringList{"per_1"});
    applyEvent(data, serverEvent("permission.replied", json(R"({"sessionID":"ses_1","requestID":"per_1","reply":"once"})")));
    QVERIFY(data.permissions.isEmpty());
    applyEvent(data, serverEvent("question.asked", json(R"({"id":"que_1","sessionID":"ses_1","questions":[]})")));
    applyEvent(data, serverEvent("question.rejected", json(R"({"sessionID":"ses_1","requestID":"que_1"})")));
    QVERIFY(data.questions.isEmpty());
}

void TestLogic::dropsADeletedSession()
{
    SessionData data;
    data.sessions.insert("ses_1", json(R"({"id":"ses_1"})"));
    data.messages.insert("ses_1", {user("msg_1", 1)});
    applyEvent(data, serverEvent("todo.updated", json(R"({"sessionID":"ses_1","todos":[{"content":"x","status":"pending"}]})")));
    QCOMPARE(data.todos["ses_1"].size(), 1);
    applyEvent(data, serverEvent("session.deleted", json(R"({"info":{"id":"ses_1"}})")));
    QVERIFY(data.sessions.isEmpty());
    QVERIFY(data.messages.isEmpty());
    QVERIFY(data.todos.isEmpty());
    // a sub-agent's session never joins the chat list
    QVERIFY(!applyEvent(data, serverEvent("session.created", json(R"({"info":{"id":"ses_2","parentID":"ses_1"}})"))).changed());
}

void TestLogic::recordsSessionErrorsInWords()
{
    SessionData data;
    applyEvent(data, serverEvent("session.error", json(R"({"sessionID":"ses_1","error":{"name":"APIError","data":{"message":"rate limited"}}})")));
    QCOMPARE(data.errors["ses_1"], QStringLiteral("rate limited"));
    QCOMPARE(errorText(json(R"({"name":"MessageAbortedError"})")), QStringLiteral("Stopped."));
}

// ---------- turns ----------

void TestLogic::groupsTheStepsOfEachPrompt()
{
    const QList<MessageEntry> entries = {user("msg_1", 1000), assistant("msg_2", "msg_1", 1100, 2000, 100),
                                         assistant("msg_3", "msg_1", 2000, 4000, 150), user("msg_4", 5000),
                                         assistant("msg_5", "msg_4", 5100)};
    const QList<Turn> turns = groupTurns(entries);
    QCOMPARE(turns.size(), 2);
    QCOMPARE(turns[0].steps.size(), 2);
    const TurnStats done = turnStats(entries, turns[0], 99999);
    QCOMPARE(done.elapsedMs, 3000.0);
    QVERIFY(!done.running);
    QCOMPARE(done.contextTokens, 150.0);
    QCOMPARE(done.tokens.input, 250.0);
    const TurnStats running = turnStats(entries, turns[1], 6000);
    QVERIFY(running.running);
    QCOMPARE(running.elapsedMs, 1000.0);
}

void TestLogic::formatsNumbers()
{
    QCOMPARE(formatTokens(999), QStringLiteral("999"));
    QCOMPARE(formatTokens(1234), QStringLiteral("1.2k"));
    QCOMPARE(formatTokens(262144), QStringLiteral("262k"));
    QCOMPARE(formatTokens(1500000), QStringLiteral("1.5M"));
    QCOMPARE(formatDuration(450), QStringLiteral("450 ms"));
    QCOMPARE(formatDuration(2100), QStringLiteral("2.1 s"));
    QCOMPARE(formatDuration(125000), QStringLiteral("2 min 5 s"));
    QCOMPARE(formatDuration(3720000), QStringLiteral("1 h 2 min"));
    QCOMPARE(formatCost(0.5), QStringLiteral("$0.50"));
    QCOMPARE(formatCost(0.0012), QStringLiteral("$0.0012"));
}

// ---------- the composer's history ----------

void TestLogic::takesTheChatsOwnPrompts()
{
    auto message = [](const QString &role, const QString &text, bool synthetic = false) {
        return MessageEntry{QJsonObject{{"role", role}},
                            {QJsonObject{{"type", "text"}, {"text", text}, {"synthetic", synthetic}}}};
    };
    const QList<MessageEntry> entries = {message("user", "list devices"), message("assistant", "samx, samy"),
                                         message("user", "read samx"), message("user", "read samx"),
                                         message("user", "added by OpenCode", true)};
    QCOMPARE(ChatLogic::promptHistory(entries, {"elsewhere"}), (QStringList{"list devices", "read samx"}));
    QCOMPARE(ChatLogic::promptHistory({}, {"a", "b"}), (QStringList{"a", "b"}));
    QCOMPARE(ChatLogic::promptHistory({message("assistant", "hi")}, {}), QStringList());
}

void TestLogic::stepsThroughEarlierPrompts()
{
    const QStringList history = {"first", "second", "third"};
    auto step = ChatLogic::recallStep(history, {}, "my draft", true);
    QVERIFY(step);
    QCOMPARE(step->text, QStringLiteral("third"));
    QCOMPARE(step->recall.index, 2);
    QCOMPARE(step->recall.draft, QStringLiteral("my draft"));
    step = ChatLogic::recallStep(history, step->recall, step->text, true);
    step = ChatLogic::recallStep(history, step->recall, step->text, true);
    QCOMPARE(step->text, QStringLiteral("first"));
    QVERIFY(!ChatLogic::recallStep(history, step->recall, step->text, true));

    const auto down = ChatLogic::recallStep(history, {1, "my draft"}, "second", false);
    QCOMPARE(down->text, QStringLiteral("third"));
    const auto back = ChatLogic::recallStep(history, down->recall, "third", false);
    QCOMPARE(back->text, QStringLiteral("my draft"));
    QCOMPARE(back->recall.index, -1);

    QVERIFY(!ChatLogic::recallStep({}, {}, "", true));
    QVERIFY(!ChatLogic::recallStep(history, {}, "", false));
}

// ---------- export ----------

void TestLogic::exportsTheChatAsMarkdown()
{
    const QJsonObject session = json(R"({"id":"ses_1","title":"Beamline health/check","time":{"created":0,"updated":0}})");
    MessageEntry prompt = user("msg_1", 1000, "Is it healthy?");
    MessageEntry answer = assistant("msg_2", "msg_1", 1100, 3100, 1200);
    answer.info.insert("modelID", "Qwen");
    QJsonObject tokens = answer.info["tokens"].toObject();
    tokens.insert("output", 40);
    answer.info.insert("tokens", tokens);
    answer.parts = {json(R"({"id":"p2","type":"reasoning","text":"Let me check.","time":{"start":1}})"),
                    json(R"({"id":"p3","type":"tool","tool":"bec_mcp_bec_status","callID":"c","state":{"status":"completed","input":{},"output":"{}","title":"","metadata":{},"time":{"start":1,"end":2}}})"),
                    json(R"({"id":"p4","type":"text","text":"Yes, all services run."})")};
    const QDateTime exported = QDateTime::fromMSecsSinceEpoch(0, QTimeZone::UTC);
    const QString markdown = ChatLogic::chatMarkdown(session, {prompt, answer}, false, exported, 99999);
    QVERIFY(markdown.contains("# Beamline health/check"));
    QVERIFY(markdown.contains("## You\n\nIs it healthy?"));
    QVERIFY(markdown.contains("- `bec_mcp_bec_status`"));
    QVERIFY(markdown.contains("Yes, all services run."));
    QVERIFY2(markdown.contains("*Qwen · 2.1 s · 1.2k in · 40 out*"), qPrintable(markdown));
    QVERIFY(!markdown.contains("Let me check."));
    QVERIFY(ChatLogic::chatMarkdown(session, {prompt, answer}, true, exported, 99999).contains("Let me check."));

    QCOMPARE(ChatLogic::exportFileName(session), QStringLiteral("Beamline-healthcheck.md"));
    QCOMPARE(ChatLogic::exportFileName({}), QStringLiteral("chat.md"));
}

void TestLogic::numbersADownloadThatExists()
{
    QTemporaryDir dir;
    const QString first = ChatLogic::freePath(dir.path(), "chat.md");
    QCOMPARE(first, dir.filePath("chat.md"));
    QFile file(first);
    QVERIFY(file.open(QIODevice::WriteOnly));
    file.close();
    QCOMPARE(ChatLogic::freePath(dir.path(), "chat.md"), dir.filePath("chat (2).md"));
}

void TestLogic::keepsADraggedWidthInTheWindow()
{
    QCOMPARE(ChatLogic::clampWidth(500.4, 280, 1600), 500);
    QCOMPARE(ChatLogic::clampWidth(100, 280, 1600), 280);
    QCOMPARE(ChatLogic::clampWidth(1500, 280, 1600), 1120);
    // a window too narrow for both still keeps the minimum
    QCOMPARE(ChatLogic::clampWidth(400, 280, 600), 280);
}

// ---------- what the thread shows ----------

void TestLogic::splitsCodeBlocksOutOfMarkdown()
{
    const auto blocks = ChatLogic::splitMarkdown("Intro\n\n```python\nprint(1)\n```\n\nOutro\n~~~\nopen");
    QCOMPARE(blocks.size(), 4);
    QVERIFY(!blocks[0].code);
    QCOMPARE(blocks[0].text, QStringLiteral("Intro"));
    QVERIFY(blocks[1].code);
    QCOMPARE(blocks[1].language, QStringLiteral("python"));
    QCOMPARE(blocks[1].text, QStringLiteral("print(1)"));
    QCOMPARE(blocks[2].text, QStringLiteral("Outro"));
    // an unclosed fence runs to the end, as while the answer streams
    QVERIFY(blocks[3].code);
    QCOMPARE(blocks[3].text, QStringLiteral("open"));
    QCOMPARE(ChatLogic::splitMarkdown("````\n```\nnested\n```\n````").first().text, QStringLiteral("```\nnested\n```"));
}

void TestLogic::describesPermissionRequests()
{
    const auto bash = ChatLogic::describePermission(
        json(R"({"permission":"bash","patterns":["rm *"],"metadata":{"command":"rm -rf build"}})"), {});
    QCOMPARE(bash.title, QStringLiteral("Run this command?"));
    QCOMPARE(bash.detail, QStringLiteral("rm -rf build"));
    const auto edit = ChatLogic::describePermission(
        json(R"({"permission":"edit","patterns":["a.py"],"metadata":{"filepath":"a.py","diff":"-x\n+y"}})"), {});
    QCOMPARE(edit.detail, QStringLiteral("a.py\n\n-x\n+y"));
    const auto other = ChatLogic::describePermission(
        json(R"({"permission":"bec_run_scan","patterns":["*"],"metadata":{}})"),
        json(R"({"tool":"bec_run_scan","state":{"input":{"scan":"line_scan"}}})"));
    QCOMPARE(other.title, QStringLiteral("Allow bec_run_scan?"));
    QCOMPARE(other.detail, QStringLiteral("{\n  \"scan\": \"line_scan\"\n}"));
}

void TestLogic::makesToolTextReadable()
{
    QCOMPARE(ChatLogic::pretty(QJsonValue(R"({"a":[1,2]})")), QStringLiteral("{\n  \"a\": [\n    1,\n    2\n  ]\n}"));
    QCOMPARE(ChatLogic::pretty(QJsonValue("plain")), QStringLiteral("plain"));
    QCOMPARE(ChatLogic::humanizeTool("bec_device_read"), QStringLiteral("Bec device read"));
    QCOMPARE(ChatLogic::childSessionID(json(R"({"state":{"metadata":{"sessionId":"ses_9"}}})")), QStringLiteral("ses_9"));
    QCOMPARE(ChatLogic::noThinkingVariant(json(R"({"variants":{"fast":{},"no-thinking":{}}})")), QStringLiteral("no-thinking"));
}

void TestLogic::opensACompactionSummaryOntoAllItsParts()
{
    MessageEntry prompt = user("msg_1", 1000, "Compact");
    MessageEntry summary = assistant("msg_2", "msg_1", 1100, 1300);
    summary.info.insert("summary", true);
    summary.parts = {json(R"({"id":"prt_r","type":"reasoning","text":"Sorting","time":{"start":1100,"end":1150}})"),
                     json(R"({"id":"prt_t","type":"tool","tool":"read","callID":"c1","state":{"status":"completed","input":{},"output":"x","time":{"start":1160,"end":1170}}})"),
                     json(R"({"id":"prt_x","type":"text","text":"So far: the broker."})")};
    QList<MessageEntry> entries = {prompt, summary};
    QHash<QString, bool> expanded;
    ThreadModel model(false);
    ThreadModel::Input input;
    input.entries = &entries;
    input.expanded = &expanded;
    model.update(input);
    QCOMPARE(kinds(model), (QStringList{"user", "summary", "footer"}));
    expanded.insert(QStringLiteral("s:msg_2"), true);
    model.update(input);
    QCOMPARE(kinds(model), (QStringList{"user", "summary", "thinking", "tool", "markdown", "footer"}));
}

void TestLogic::buildsTheThreadsRows()
{
    MessageEntry prompt = user("msg_1", 1000, "Explain");
    MessageEntry step = assistant("msg_2", "msg_1", 1100);
    step.parts = {json(R"({"id":"prt_r","type":"reasoning","text":"Hmm","time":{"start":1100}})"),
                  json(R"({"id":"prt_t","type":"tool","tool":"bash","callID":"c1","state":{"status":"running","input":{"command":"ls"},"time":{"start":1200}}})"),
                  json(R"({"id":"prt_x","type":"text","text":"Look:\n\n```sh\nls\n```"})")};
    QList<MessageEntry> entries = {prompt, step};
    QHash<QString, bool> expanded;
    ThreadModel model(false);
    ThreadModel::Input input;
    input.entries = &entries;
    input.busy = true;
    input.expanded = &expanded;
    input.permissions = {json(R"({"id":"per_1","tool":{"callID":"c1"}})")};
    model.update(input);
    QCOMPARE(kinds(model), (QStringList{"user", "thinking", "tool", "markdown", "code", "footer"}));
    const auto &rows = model.rows();
    // the thinking runs and is open; the tool waits for approval and opens for it
    QVERIFY(rows[1].item["running"].toBool());
    QVERIFY(rows[1].item["expanded"].toBool());
    QVERIFY(rows[2].item["waiting"].toBool());
    QVERIFY(rows[2].item["expanded"].toBool());
    QCOMPARE(rows[2].item["summary"].toString(), QStringLiteral("Waiting for your approval"));
    QVERIFY(rows[5].item["live"].toBool());

    // a streaming delta changes its row (and the footer's copy of the answer) and moves none
    QSignalSpy inserted(&model, &QAbstractItemModel::rowsInserted);
    QSignalSpy removed(&model, &QAbstractItemModel::rowsRemoved);
    QSignalSpy changed(&model, &QAbstractItemModel::dataChanged);
    entries[1].parts[2].insert("text", "Look:\n\n```sh\nls -la\n```");
    model.update(input);
    QCOMPARE(inserted.size(), 0);
    QCOMPARE(removed.size(), 0);
    QCOMPARE(changed.size(), 2);
    QCOMPARE(changed[0].first().toModelIndex().row(), 4);
    QCOMPARE(changed[1].first().toModelIndex().row(), 5);

    // the answer ends: the user closes the thinking, the tool completes, the footer shows actions
    expanded.insert("prt_r", false);
    entries[1].parts[0].insert("time", json(R"({"start":1100,"end":1500})"));
    entries[1].parts[1].insert("state", json(R"({"status":"completed","input":{"command":"ls"},"output":"a\nb","time":{"start":1200,"end":1300}})"));
    entries[1].info.insert("time", json(R"({"created":1100,"completed":2000})"));
    input.busy = false;
    input.permissions.clear();
    model.update(input);
    QVERIFY(!model.rows()[1].item["expanded"].toBool());
    QCOMPARE(model.rows()[2].item["output"].toString(), QStringLiteral("a\nb"));
    QVERIFY(!model.rows()[5].item["live"].toBool());
    QCOMPARE(model.rows()[5].item["end"].toDouble(), 2000.0);

    // undoing from the prompt marks it and everything after
    input.revertID = "msg_1";
    model.update(input);
    QCOMPARE(keys(model).first(), QStringLiteral("undo"));
    QVERIFY(model.rows()[1].item["undone"].toBool());
}

void TestLogic::showsAnEmptyChatAndASubAgentsSteps()
{
    ThreadModel thread(false);
    thread.update({});
    QCOMPARE(kinds(thread), QStringList{"empty"});

    ThreadModel steps(true);
    steps.update({});
    QCOMPARE(kinds(steps), QStringList{"loading"});
    QList<MessageEntry> entries = {user("msg_1", 1)};
    ThreadModel::Input input;
    input.entries = &entries;
    steps.update(input);
    QCOMPARE(kinds(steps), QStringList{"note"});
    MessageEntry step = assistant("msg_2", "msg_1", 2);
    step.parts = {json(R"({"id":"prt_x","type":"text","text":"Done."})")};
    step.info.insert("error", json(R"({"name":"MessageAbortedError"})"));
    entries << step;
    steps.update(input);
    QCOMPARE(kinds(steps), (QStringList{"markdown", "stopped"}));
}

// ---------- bob's settings and containers ----------

void TestLogic::readsAndSavesSettings()
{
    QTemporaryDir dir;
    const QString path = dir.filePath("settings.json");
    BobSettingsStore fresh(path);
    QCOMPARE(fresh.get(), BobSettings());
    QVERIFY(fresh.get().autoJoin);
    QCOMPARE(fresh.set({"  /w/bob.toml ", " ", false}), QString());
    QFile file(path);
    QVERIFY(file.open(QIODevice::ReadOnly));
    // the Tauri app's keys: unset paths are null
    QCOMPARE(QJsonDocument::fromJson(file.readAll()).object(),
             json(R"({"config":"/w/bob.toml","binary":null,"autoJoin":false})"));
    file.close();
    BobSettingsStore again(path);
    QCOMPARE(again.get().config, QStringLiteral("/w/bob.toml"));
    QVERIFY(again.get().binary.isEmpty());
    QVERIFY(!again.get().autoJoin);

    QVERIFY(file.open(QIODevice::WriteOnly));
    file.write("{broken");
    file.close();
    QTest::ignoreMessage(QtWarningMsg, QRegularExpression("ignoring .*settings.json"));
    QCOMPARE(BobSettingsStore(path).get(), BobSettings());
    QVERIFY(file.open(QIODevice::WriteOnly));
    file.write(R"({"config":"/x/bob.toml"})");
    file.close();
    QVERIFY(BobSettingsStore(path).get().autoJoin);
}

void TestLogic::readsAConnectionFromInspect()
{
    auto inspect = [](const char *hostIp) {
        QJsonObject entry = json(R"({
            "Name": "/bob-501-abc",
            "Config": {"Labels": {"io.bob.launch": "x"}, "Env": ["A=1", "OPENCODE_SERVER_PASSWORD=pw=1"],
                       "WorkingDir": "/workspace", "Image": "localhost/bob"},
            "State": {"StartedAt": "2026-10-21T12:00:00.123456789Z"},
            "NetworkSettings": {"Ports": {"4096/tcp": [{"HostIp": "IP", "HostPort": "52997"}]}}})");
        QJsonObject network = entry["NetworkSettings"].toObject();
        QJsonObject ports = network["Ports"].toObject();
        ports.insert("4096/tcp", QJsonArray{QJsonObject{{"HostIp", hostIp}, {"HostPort", "52997"}}});
        network.insert("Ports", ports);
        entry.insert("NetworkSettings", network);
        return entry;
    };
    const auto bob = runningBob(inspect("0.0.0.0"), "docker");
    QVERIFY(bob);
    QCOMPARE(bob->url, QStringLiteral("http://127.0.0.1:52997"));
    QCOMPARE(bob->password, QStringLiteral("pw=1"));
    QCOMPARE(bob->container, QStringLiteral("bob-501-abc"));
    QCOMPARE(bob->runtime, QStringLiteral("docker"));
    QCOMPARE(bob->workspace, QStringLiteral("/workspace"));
    QCOMPARE(runningBob(inspect("::"), "podman")->url, QStringLiteral("http://[::1]:52997"));
    QCOMPARE(runningBob(inspect("fe80::1"), "podman")->url, QStringLiteral("http://[fe80::1]:52997"));
    QCOMPARE(runningBob(inspect("10.0.0.2"), "podman")->url, QStringLiteral("http://10.0.0.2:52997"));

    QJsonObject other = inspect("0.0.0.0");
    QJsonObject config = other["Config"].toObject();
    config.insert("Labels", QJsonObject());
    other.insert("Config", config);
    QVERIFY(!runningBob(other, "docker"));
}

QTEST_GUILESS_MAIN(TestLogic)
