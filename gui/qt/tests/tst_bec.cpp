// The BEC plugin's cards and scan calls, against recorded tool results (the plugin's fixtures).
#include "beccards.h"

#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QStringList>
#include <QTest>

using namespace BecCards;

namespace {
QString output(const QString &name)
{
    QFile file(QStringLiteral(BEC_FIXTURES "/%1.json").arg(name));
    if (!file.open(QIODevice::ReadOnly))
        qFatal("no fixture %s", qPrintable(name));
    return QString::fromUtf8(file.readAll());
}

QJsonObject card(const QString &name)
{
    return buildCard(name, parseToolResult(output(name)));
}

QJsonObject json(const char *text)
{
    return QJsonDocument::fromJson(text).object();
}

QJsonObject find(const QJsonArray &list, const std::function<bool(const QJsonObject &)> &match)
{
    for (const QJsonValue &item : list) {
        if (match(item.toObject()))
            return item.toObject();
    }
    return {};
}
} // namespace

class TestBec : public QObject
{
    Q_OBJECT

private slots:
    void readsTheServersJsonText()
    {
        QVERIFY(!parseToolResult(output("bec_queue")).value("summary").toString().isEmpty());
        QVERIFY(parseToolResult(QStringLiteral("total 8\ndrwxr-xr-x")).isEmpty());
        QVERIFY(parseToolResult(QStringLiteral("{\"no\":\"summary\"}")).isEmpty());
        QVERIFY(parseToolResult(QStringLiteral("[1,2]")).isEmpty());
        QVERIFY(!parseToolResult(json("{\"summary\": \"structured\"}")).isEmpty());
    }

    void drawsEveryFixture_data()
    {
        QTest::addColumn<QString>("name");
        for (const char *name : {"bec_beamline_states", "bec_code_read", "bec_code_search", "bec_code_traceback",
                                 "bec_device_diagnose", "bec_device_read", "bec_devices", "bec_diagnose", "bec_docs_search",
                                 "bec_log_analysis", "bec_log_range", "bec_logs", "bec_queue", "bec_scan_help",
                                 "bec_scan_history", "bec_scans", "bec_status", "mcp_status"})
            QTest::newRow(name) << QString::fromLatin1(name);
    }
    void drawsEveryFixture()
    {
        QFETCH(QString, name);
        const QJsonObject result = parseToolResult(output(name));
        QVERIFY(!result.isEmpty());
        QCOMPARE(buildCard(name, result).value("summary"), result.value("summary"));
    }

    void showsADeviceReadingAgainstItsLimits()
    {
        const QJsonObject reading = card("bec_device_read");
        const QJsonObject gauge = reading.value("gauge").toObject();
        QCOMPARE(gauge.value("label").toString(), QString("samx"));
        QCOMPARE(gauge.value("value").toString(), QString("5.005 mm"));
        QCOMPARE(gauge.value("low").toString(), QString("-50"));
        QVERIFY(gauge.value("fraction").toDouble() > 0.5);
        QCOMPARE(reading.value("table").toObject().value("columns").toArray(), QJsonArray({"Signal", "Value", "Age"}));
    }

    void listsDevicesWithANoteWhenThePageIsPartial()
    {
        const QJsonObject devices = card("bec_devices");
        const QJsonObject table = devices.value("table").toObject();
        QCOMPARE(table.value("rows").toArray().size(), 5);
        QCOMPARE(table.value("note").toString(), QString("Showing 1–5 of 216"));
        QVERIFY(!find(devices.value("stats").toArray(), [](const QJsonObject &s) { return s.value("label") == "positioner"; }).isEmpty());
    }

    void splitsFindingsIntoTextAndNextStep()
    {
        const QJsonObject diagnosis = buildCard("bec_diagnose", json("{\"ok\": true, \"summary\": \"s\", \"findings\": [\"Warning: the queue is paused. Next: bec_queue().\"]}"));
        const QJsonObject finding = diagnosis.value("findings").toArray().first().toObject();
        QCOMPARE(finding.value("severity").toString(), QString("warning"));
        QCOMPARE(finding.value("tone").toString(), QString("warn"));
        QCOMPARE(finding.value("text").toString(), QString("the queue is paused."));
        QCOMPARE(finding.value("next").toString(), QString("bec_queue()"));
        QCOMPARE(diagnosis.value("tone").toString(), QString("warn"));
        QCOMPARE(buildCard("bec_status", json("{\"ok\": false, \"summary\": \"BEC is down\"}")).value("tone").toString(), QString("error"));
    }

    void keepsTheSummaryForAShapeItDoesNotKnow()
    {
        const QJsonObject unknown = buildCard("bec_new_tool", json("{\"summary\": \"fine\", \"data\": {\"x\": 1}}"));
        QCOMPARE(unknown.value("title").toString(), QString("bec new tool"));
        QCOMPARE(unknown.value("summary").toString(), QString("fine"));
        QVERIFY(title("bec_new_tool").isEmpty());
    }

    // Keyword arguments come out in Qt's key order (alphabetical), not the order they were sent in.
    void formatsScanCallsAsTypedInTheBecClient()
    {
        QCOMPARE(formatScanCall(json("{\"scan\": \"line_scan\", \"args\": [\"samx\", -1, 1], \"kwargs\": {\"steps\": 10, \"relative\": true}}")),
                 QString("scans.line_scan(dev.samx, -1, 1, relative=True, steps=10)"));
        QCOMPARE(formatScanCall(json("{\"scan_name\": \"acquire\", \"args\": \"[]\", \"kwargs\": \"{\\\"exp_time\\\": 0.1}\"}")),
                 QString("scans.acquire(exp_time=0.1)"));
        QCOMPARE(formatScanCall(json("{\"scan\": \"line_scan\", \"args\": [\"samx\", -1, 1], \"kwargs\": {\"steps\": 10, \"relative\": false, \"name\": \"it's\"}}")),
                 QString("scans.line_scan(dev.samx, -1, 1, name='it\\'s', relative=False, steps=10)"));
        QCOMPARE(formatScanCall(json("{\"scan\": \"grid_scan\", \"args\": [\"samx\", -2, 2, 5], \"kwargs\": {\"md\": {\"sample\": \"Fe\"}, \"positions\": null}}")),
                 QString("scans.grid_scan(dev.samx, -2, 2, 5, md={'sample': 'Fe'}, positions=None)"));
    }

    void wordsTheSubmissionResult()
    {
        QCOMPARE(scanResultCard(json("{\"submitted\": true, \"status\": \"running\", \"scan_number\": 12}"), "scans.acquire()")
                     .value("summary").toString(),
                 QString("Scan #12 is running."));
        QCOMPARE(scanResultCard(json("{\"submitted\": true, \"accepted\": false, \"guard_message\": \"locked\"}"), "x")
                     .value("tone").toString(),
                 QString("error"));
    }

    void drawsAToolRowForEachState()
    {
        QCOMPARE(toolView("bec_queue", "running", QJsonObject(), QJsonValue()).value("label").toString(), QString("Checking the scan queue"));
        QVERIFY(toolView("bec_queue", "error", QJsonObject(), QJsonValue()).value("card").isUndefined());
        const QJsonObject done = toolView("bec_queue", "completed", QJsonObject(), output("bec_queue"));
        QCOMPARE(done.value("label").toString(), QString("Scan queue"));
        QCOMPARE(done.value("card").toObject().value("kind").toString(), QString("bec_queue"));
        const QJsonObject input = json("{\"scan\": \"acquire\", \"args\": [], \"kwargs\": {}}");
        const QJsonObject scan = toolView("bec_run_scan", "completed", input, QString("{\"submitted\": true, \"scan_number\": 3}"));
        QCOMPARE(scan.value("summary").toString(), QString("Scan #3 is in the queue."));
        QCOMPARE(scan.value("card").toObject().value("code").toObject().value("text").toString(), QString("scans.acquire()"));
        QCOMPARE(toolView("bec_run_scan", "completed", input, QString("not json")).value("summary").toString(), QString("scans.acquire()"));
        QVERIFY(toolView("unknown_tool", "completed", QJsonObject(), QString("plain text")).isEmpty());
    }

    void groupsTheLogAnalysisIntoIncidents()
    {
        const QJsonObject analysis = card("bec_log_analysis");
        QVERIFY(!analysis.value("incidents").toArray().isEmpty());
        const QJsonObject limits = find(analysis.value("incidents").toArray(), [](const QJsonObject &i) {
            return i.value("title").toString().startsWith("DeviceInstructionError");
        });
        QVERIFY(limits.value("likelyCause").toString().contains("software limits"));
        QCOMPARE(limits.value("devices").toArray(), QJsonArray({"samx"}));
        QCOMPARE(limits.value("origin").toObject().value("where").toString(),
                 QString("bec_server/scan_server/scans/scan_status.py:183 (_raise_if_failed)"));
        QCOMPARE(limits.value("tracebackRecord").toInt(), 592);
        QVERIFY(!limits.value("context").toArray().isEmpty());
        const QJsonObject cached = find(analysis.value("stats").toArray(), [](const QJsonObject &s) { return s.value("label") == "records cached"; });
        QCOMPARE(cached.value("value").toString(), QString("760 / 3000"));
    }

    void listsLogRecordsWithTheirTracebacks()
    {
        const QJsonObject range = card("bec_log_range");
        QList<int> ids;
        for (const QJsonValue &record : range.value("records").toArray())
            ids << record.toObject().value("id").toInt();
        QCOMPARE(ids, QList<int>({589, 590, 591, 592, 593, 594}));
        const QJsonObject failed = find(range.value("records").toArray(), [](const QJsonObject &r) { return r.value("id").toInt() == 589; });
        QCOMPARE(failed.value("tone").toString(), QString("error"));
        QVERIFY(failed.value("traceback").toObject().value("origin").toString().contains("scan_status.py:183"));
        QCOMPARE(range.value("badges").toArray().first().toObject().value("text").toString(), QString("records 589–594"));
    }

    void showsTracebackFramesWithTheRaisingLineMarked()
    {
        const QJsonObject traceback = card("bec_code_traceback");
        const QJsonArray frames = traceback.value("frames").toArray();
        QCOMPARE(frames.size(), 12);
        const QJsonObject last = frames.at(11).toObject();
        QCOMPARE(last.value("function").toString(), QString("DirectScanWorker._handle_exception"));
        bool marked = false;
        for (const QString &line : last.value("code").toString().split('\n'))
            marked = marked || (line.startsWith('>') && line.contains("raise ScanAbortion"));
        QVERIFY(marked);
        QCOMPARE(traceback.value("exceptions").toArray().at(1).toObject().value("relation").toString(), QString("cause"));
    }

    void showsCodeAndSearchMatches()
    {
        QVERIFY(card("bec_code_read").value("code").toObject().value("label").toString().contains("ScanStatus._raise_if_failed"));
        const QJsonArray rows = card("bec_code_search").value("table").toObject().value("rows").toArray();
        QCOMPARE(rows.first().toArray().first().toObject().value("text").toString(),
                 QString("ophyd_devices/ophyd_devices/sim/sim_positioner.py:174"));
    }
};

QTEST_GUILESS_MAIN(TestBec)
#include "tst_bec.moc"
