#include "beccards.h"

#include <QHash>
#include <QJSValue>
#include <QJsonArray>
#include <QJsonDocument>
#include <QRegularExpression>

#include <algorithm>
#include <cmath>
#include <functional>

namespace {

using Card = QJsonObject;

const QHash<QString, QString> toneForStatus{
    {"RUNNING", "ok"}, {"COMPLETED", "ok"}, {"closed", "ok"}, {"IDLE", "ok"}, {"PENDING", "info"},
    {"PAUSED", "warn"}, {"LOCKED", "warn"}, {"HALTED", "warn"}, {"user_completed", "info"},
    {"STOPPED", "error"}, {"ERROR", "error"}, {"aborted", "error"}, {"halted", "warn"}, {"BUSY", "info"}};
const QHash<QString, QString> severityTone{{"error", "error"}, {"warning", "warn"}, {"info", "info"}};
const QHash<QString, QString> levelTone{{"ERROR", "error"}, {"CRITICAL", "error"}, {"WARNING", "warn"},
                                        {"SUCCESS", "ok"},   {"INFO", "neutral"},   {"DEBUG", "neutral"}};

const QHash<QString, QString> titles{
    {"bec_status", "BEC services"}, {"bec_ping", "Connection"}, {"bec_devices", "Devices"},
    {"bec_device_info", "Device"}, {"bec_device_read", "Reading"}, {"bec_queue", "Scan queue"},
    {"bec_beamline_states", "Beamline states"}, {"bec_scans", "Available scans"}, {"bec_scan_help", "Scan"},
    {"bec_scan_history", "Recent scans"}, {"bec_logs", "Service logs"}, {"bec_diagnose", "Beamline diagnosis"},
    {"bec_device_diagnose", "Device diagnosis"}, {"bec_docs_search", "Documentation"},
    {"bec_docs_page", "Documentation"}, {"mcp_status", "MCP server"}, {"bec_run_scan", "Scan"},
    {"bec_log_range", "Log records"}, {"bec_log_analysis", "Log analysis"}, {"bec_code_traceback", "Traceback"},
    {"bec_code_read", "Code"}, {"bec_code_search", "Code search"}, {"bec_queue_control", "Queue control"},
    {"bec_service_control", "Service control"}};

// What the tool row says while the call runs, in the user's words.
const QHash<QString, QString> activities{
    {"bec_status", "Checking BEC services"}, {"bec_ping", "Pinging BEC"}, {"bec_devices", "Listing devices"},
    {"bec_device_info", "Looking up the device"}, {"bec_device_read", "Reading the device"},
    {"bec_queue", "Checking the scan queue"}, {"bec_beamline_states", "Checking beamline states"},
    {"bec_scans", "Listing scans"}, {"bec_scan_help", "Looking up the scan"},
    {"bec_scan_history", "Reading the scan history"}, {"bec_logs", "Reading service logs"},
    {"bec_diagnose", "Diagnosing the beamline"}, {"bec_device_diagnose", "Diagnosing the device"},
    {"bec_docs_search", "Searching the docs"}, {"bec_docs_page", "Reading the docs"},
    {"mcp_status", "Checking the MCP server"}, {"bec_run_scan", "Running the scan"},
    {"bec_log_range", "Reading the log"}, {"bec_log_analysis", "Analysing the log"},
    {"bec_code_traceback", "Reading the code behind the traceback"}, {"bec_code_read", "Reading code"},
    {"bec_code_search", "Searching the code"}, {"bec_queue_control", "Controlling the queue"},
    {"bec_service_control", "Controlling the services"}};

// --- the server's JSON, read the way its Python-written results are meant -----------------------

bool given(const QJsonValue &value)
{
    return !value.isNull() && !value.isUndefined();
}

bool truthy(const QJsonValue &value)
{
    switch (value.type()) {
    case QJsonValue::Bool:
        return value.toBool();
    case QJsonValue::Double:
        return value.toDouble() != 0 && !std::isnan(value.toDouble());
    case QJsonValue::String:
        return !value.toString().isEmpty();
    case QJsonValue::Array:
    case QJsonValue::Object:
        return true;
    default:
        return false;
    }
}

// `value`, or `fallback` when it is empty, zero, false or missing
QJsonValue orElse(const QJsonValue &value, const QJsonValue &fallback)
{
    return truthy(value) ? value : fallback;
}

QString number(double value)
{
    if (std::isnan(value))
        return QStringLiteral("NaN");
    if (std::isinf(value))
        return value > 0 ? QStringLiteral("Infinity") : QStringLiteral("-Infinity");
    if (value == 0)
        return QStringLiteral("0");
    const double size = std::abs(value);
    if (value == std::floor(value) && size < 1e21)
        return QString::number(value, 'f', 0);
    return QString::number(value, size < 1e-6 || size >= 1e21 ? 'g' : 'f', QLocale::FloatingPointShortest);
}

QString str(const QJsonValue &value);

QString join(const QJsonArray &list, const QString &separator, const std::function<QString(const QJsonValue &)> &format)
{
    QStringList parts;
    for (const QJsonValue &item : list)
        parts << format(item);
    return parts.join(separator);
}

// A value as text, as the server's own String() would write it
QString str(const QJsonValue &value)
{
    switch (value.type()) {
    case QJsonValue::String:
        return value.toString();
    case QJsonValue::Double:
        return number(value.toDouble());
    case QJsonValue::Bool:
        return value.toBool() ? QStringLiteral("true") : QStringLiteral("false");
    case QJsonValue::Null:
        return QStringLiteral("null");
    case QJsonValue::Array:
        return join(value.toArray(), QStringLiteral(","), [](const QJsonValue &item) { return given(item) ? str(item) : QString(); });
    case QJsonValue::Object:
        return QStringLiteral("[object Object]");
    default:
        return QStringLiteral("undefined");
    }
}

double toNumber(const QJsonValue &value)
{
    switch (value.type()) {
    case QJsonValue::Double:
        return value.toDouble();
    case QJsonValue::Bool:
        return value.toBool() ? 1 : 0;
    case QJsonValue::Null:
        return 0;
    case QJsonValue::String: {
        const QString text = value.toString().trimmed();
        if (text.isEmpty())
            return 0;
        bool ok = false;
        const double parsed = text.toDouble(&ok);
        return ok ? parsed : std::nan("");
    }
    default:
        return std::nan("");
    }
}

QString fixed(double value, int decimals)
{
    return QString::number(value, 'f', decimals);
}

// A value as a table cell or a stat shows it
QString text(const QJsonValue &value)
{
    switch (value.type()) {
    case QJsonValue::Null:
    case QJsonValue::Undefined:
        return QStringLiteral("–");
    case QJsonValue::Bool:
        return value.toBool() ? QStringLiteral("yes") : QStringLiteral("no");
    case QJsonValue::Double: {
        const double d = value.toDouble();
        return d == std::floor(d) ? number(d) : number(QString::number(d, 'g', 6).toDouble());
    }
    case QJsonValue::Array: {
        const QString joined = join(value.toArray(), QStringLiteral(", "), text);
        return joined.isEmpty() ? QStringLiteral("–") : joined;
    }
    case QJsonValue::Object:
        return QString::fromUtf8(QJsonDocument(value.toObject()).toJson(QJsonDocument::Compact));
    default:
        return value.toString();
    }
}

QString underscoresToSpaces(QString name)
{
    return name.replace(QLatin1Char('_'), QLatin1Char(' '));
}

QJsonObject cell(const QJsonValue &value, const QString &tone = {})
{
    return {{"text", text(value)}, {"tone", tone}};
}

QString duration(const QJsonValue &seconds)
{
    const double s = toNumber(seconds);
    if (!given(seconds) || std::isnan(s))
        return QStringLiteral("–");
    if (s < 1)
        return fixed(s * 1000, 0) + QStringLiteral(" ms");
    if (s < 90)
        return fixed(s, 1) + QStringLiteral(" s");
    if (s < 5400)
        return fixed(s / 60, 0) + QStringLiteral(" min");
    if (s < 172800)
        return fixed(s / 3600, 1) + QStringLiteral(" h");
    return fixed(s / 86400, 1) + QStringLiteral(" d");
}

QString toneFor(const QJsonValue &status)
{
    return toneForStatus.value(str(status), QStringLiteral("neutral"));
}

QJsonObject object(const QJsonValue &value)
{
    return value.toObject();
}

QJsonArray array(const QJsonValue &value)
{
    return value.toArray();
}

void append(Card &card, const QString &key, const QJsonValue &item)
{
    QJsonArray list = card.value(key).toArray();
    list.append(item);
    card.insert(key, list);
}

QJsonObject table(const QStringList &columns, const QJsonArray &rows, const QString &note = {})
{
    return {{"columns", QJsonArray::fromStringList(columns)}, {"rows", rows}, {"note", note}};
}

Card emptyCard(const QString &kind, const QString &title, const QString &summary, const QString &tone)
{
    return {{"kind", kind},          {"title", title},       {"summary", summary}, {"tone", tone},
            {"badges", QJsonArray()}, {"stats", QJsonArray()}, {"findings", QJsonArray()},
            {"links", QJsonArray()},  {"lines", QJsonArray()}};
}

// --- the cards ---------------------------------------------------------------------------------

QJsonArray findings(const QJsonObject &answer)
{
    const QJsonArray texts = array(answer.value("findings"));
    const QJsonArray structured = array(object(answer.value("data")).value("findings"));
    QJsonArray list;
    for (qsizetype index = 0; index < texts.size(); ++index) {
        const QJsonObject info = object(structured.at(index));
        const QString raw = str(texts.at(index));
        QString severity = truthy(info.value("severity")) ? str(info.value("severity"))
                                                          : raw.section(QLatin1Char(':'), 0, 0).trimmed().toLower();
        if (!severityTone.contains(severity))
            severity = QStringLiteral("info");
        const qsizetype split = raw.indexOf(QStringLiteral(" Next: "));
        QString body = split == -1 ? raw : raw.left(split);
        QString next = split == -1 ? QString() : raw.mid(split + 7);
        if (next.endsWith(QLatin1Char('.')))
            next.chop(1);
        next = next.trimmed();
        const QString prefix = severity.left(1).toUpper() + severity.mid(1) + QStringLiteral(": ");
        if (body.startsWith(prefix))
            body = body.mid(prefix.size());
        list.append(QJsonObject{{"severity", severity}, {"tone", severityTone.value(severity)},
                                {"text", body.trimmed()}, {"next", next}});
    }
    return list;
}

QString overallTone(const QJsonArray &list, bool ok)
{
    if (!ok)
        return QStringLiteral("error");
    bool warning = false;
    for (const QJsonValue &finding : list) {
        const QString severity = finding.toObject().value("severity").toString();
        if (severity == QStringLiteral("error"))
            return QStringLiteral("error");
        warning = warning || severity == QStringLiteral("warning");
    }
    return warning ? QStringLiteral("warn") : QStringLiteral("ok");
}

void statusCard(Card &card, const QJsonObject &data)
{
    const QJsonObject redis = object(data.value("redis"));
    if (!redis.isEmpty()) {
        const QJsonValue rtt = redis.value("round_trip_ms");
        append(card, "badges", QJsonObject{
            {"text", QStringLiteral("Redis ") + str(orElse(redis.value("address"), QString()))
                         + (rtt.isDouble() ? QStringLiteral(" · ") + fixed(rtt.toDouble(), 1) + QStringLiteral(" ms") : QString())},
            {"tone", truthy(redis.value("answered")) ? "ok" : "error"}});
    }
    QJsonArray services = array(data.value("core_services"));
    for (const QJsonValue &service : array(data.value("other_services")))
        services.append(service);
    QJsonArray rows;
    for (const QJsonValue &entry : services) {
        const QJsonObject s = entry.toObject();
        rows.append(QJsonArray{cell(s.value("name")), cell(s.value("status"), toneFor(s.value("status"))),
                               cell(fixed(toNumber(orElse(s.value("cpu_percent"), 0)), 1) + QStringLiteral(" %")),
                               cell(fixed(toNumber(orElse(s.value("memory_mb"), 0)), 0) + QStringLiteral(" MB")),
                               cell(duration(s.value("uptime_s"))), cell(s.value("bec_lib"))});
    }
    if (!rows.isEmpty())
        card.insert("table", table({"Service", "Status", "CPU", "Memory", "Uptime", "bec_lib"}, rows));
    const QJsonObject clients = object(data.value("clients"));
    if (given(clients.value("count")))
        append(card, "stats", QJsonObject{{"label", "Clients"}, {"value", str(clients.value("count"))}});
}

void pingCard(Card &card, const QJsonObject &data)
{
    if (data.value("round_trip_ms").isDouble())
        append(card, "stats", QJsonObject{{"label", "round trip"}, {"value", fixed(data.value("round_trip_ms").toDouble(), 1) + " ms"}});
    if (!data.value("services").isUndefined())
        append(card, "stats", QJsonObject{{"label", "services"}, {"value", text(data.value("services"))}});
}

void devicesCard(Card &card, const QJsonObject &data)
{
    const QJsonObject byType = object(data.value("by_type"));
    for (auto it = byType.begin(); it != byType.end(); ++it)
        append(card, "stats", QJsonObject{{"label", underscoresToSpaces(it.key())}, {"value", str(it.value())}});
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("devices"))) {
        const QJsonObject d = entry.toObject();
        const bool enabled = truthy(d.value("enabled"));
        rows.append(QJsonArray{cell(d.value("name"), "accent"), cell(d.value("type")), cell(d.value("device_class")),
                               cell(d.value("readout_priority")),
                               cell(enabled ? "enabled" : "disabled", enabled ? "ok" : "warn")});
    }
    QString note;
    const QJsonValue total = data.value("total");
    const QJsonValue returned = data.value("returned");
    if (total.isDouble() && returned.isDouble() && total.toDouble() > returned.toDouble()) {
        const double start = toNumber(orElse(data.value("offset"), 0)) + 1;
        note = QStringLiteral("Showing %1–%2 of %3")
                   .arg(number(start), number(start + returned.toDouble() - 1), number(total.toDouble()));
    }
    card.insert("table", table({"Name", "Type", "Class", "Readout", "State"}, rows, note));
}

void deviceReadCard(Card &card, const QJsonObject &data)
{
    const QString name = str(orElse(data.value("name"), QString()));
    const QString egu = str(orElse(data.value("egu"), QString()));
    const QJsonObject readback = object(data.value("readback"));
    QJsonObject main = object(readback.value(name));
    if (!truthy(readback.value(name)))
        main = readback.isEmpty() ? QJsonObject() : object(readback.begin().value());
    const QJsonValue value = main.value("value");
    const QJsonObject limits = object(data.value("limits"));
    const QJsonValue low = limits.value("low");
    const QJsonValue high = limits.value("high");
    if (value.isDouble()) {
        QJsonObject gauge{{"label", name},
                          {"value", (text(value) + QLatin1Char(' ') + egu).trimmed()},
                          {"age", truthy(main.value("age_s")) ? "updated " + duration(main.value("age_s")) + " ago" : QString()},
                          {"low", ""}, {"high", ""}, {"fraction", -1}, {"tone", "ok"}};
        if (low.isDouble() && high.isDouble() && high.toDouble() > low.toDouble()) {
            const double fraction = (value.toDouble() - low.toDouble()) / (high.toDouble() - low.toDouble());
            gauge.insert("low", text(low));
            gauge.insert("high", text(high));
            gauge.insert("fraction", std::clamp(fraction, 0.0, 1.0));
            if (fraction <= 0.02 || fraction >= 0.98)
                gauge.insert("tone", "warn");
        }
        card.insert("gauge", gauge);
    }
    QJsonArray rows;
    const auto addSignals = [&](const QJsonObject &signalMap) {
        for (auto it = signalMap.begin(); it != signalMap.end(); ++it) {
            const QJsonObject info = object(it.value());
            rows.append(QJsonArray{cell(it.key()), cell(info.value("value"), it.key() == name ? "accent" : ""),
                                   cell(duration(info.value("age_s")))});
        }
    };
    addSignals(readback);
    addSignals(object(data.value("configuration")));
    if (!rows.isEmpty())
        card.insert("table", table({"Signal", "Value", "Age"}, rows));
    const bool enabled = given(data.value("enabled")) ? truthy(data.value("enabled")) : true;
    append(card, "badges", QJsonObject{{"text", enabled ? "enabled" : "disabled"}, {"tone", enabled ? "ok" : "warn"}});
    if (truthy(data.value("type")))
        append(card, "badges", QJsonObject{{"text", str(data.value("type"))}, {"tone", "neutral"}});
}

void deviceInfoCard(Card &card, const QJsonObject &data)
{
    QJsonArray rows;
    for (const char *key : {"name", "type", "device_class", "readout_priority", "enabled", "description"}) {
        if (data.contains(QLatin1String(key)))
            rows.append(QJsonArray{cell(underscoresToSpaces(QString::fromLatin1(key))), cell(data.value(QLatin1String(key)))});
    }
    if (!rows.isEmpty())
        card.insert("table", table({"Field", "Value"}, rows));
}

void queueCard(Card &card, const QJsonObject &data)
{
    const QJsonObject queues = object(data.value("queues"));
    for (auto it = queues.begin(); it != queues.end(); ++it) {
        const QJsonObject queue = object(it.value());
        append(card, "badges", QJsonObject{{"text", it.key() + ": " + str(queue.value("status"))}, {"tone", toneFor(queue.value("status"))}});
        append(card, "stats", QJsonObject{{"label", it.key() + " pending"}, {"value", str(orElse(queue.value("size"), 0))}});
        for (const QJsonValue &lock : array(queue.value("locks")))
            append(card, "findings", QJsonObject{{"severity", "warning"}, {"tone", "warn"}, {"text", "Locked: " + text(lock)}, {"next", ""}});
    }
    QJsonArray rows;
    const QJsonObject running = object(data.value("running"));
    if (truthy(data.value("running"))) {
        rows.append(QJsonArray{
            cell(text(running.contains("scan_numbers") ? running.value("scan_numbers") : running.value("scan_number"))),
            cell(text(running.contains("requests") ? running.value("requests") : running.value("scan_name"))),
            cell(orElse(running.value("status"), QStringLiteral("RUNNING")), "info"), cell("now")});
    }
    for (const QJsonValue &entry : array(data.value("history"))) {
        const QJsonObject item = entry.toObject();
        rows.append(QJsonArray{cell(text(item.value("scan_numbers"))), cell(text(item.value("requests"))),
                               cell(item.value("status"), toneFor(item.value("status"))),
                               cell(orElse(item.value("reason"), QString()))});
    }
    if (!rows.isEmpty())
        card.insert("table", table({"Scan #", "Request", "Status", "Note"}, rows));
}

void historyCard(Card &card, const QJsonObject &data)
{
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("scans"))) {
        const QJsonObject scan = entry.toObject();
        QString started = str(orElse(scan.value("start_time"), QString())).left(19);
        const qsizetype t = started.indexOf(QLatin1Char('T'));
        if (t != -1)
            started[t] = QLatin1Char(' ');
        rows.append(QJsonArray{cell(scan.value("scan_number"), "accent"), cell(scan.value("scan_name")),
                               cell(scan.value("status"), toneFor(scan.value("status"))),
                               cell(duration(scan.value("duration_s"))), cell(scan.value("num_points")), cell(started)});
    }
    card.insert("table", table({"#", "Scan", "Status", "Duration", "Points", "Started"}, rows));
}

void scansCard(Card &card, const QJsonObject &data)
{
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("scans"))) {
        const QJsonObject scan = entry.toObject();
        rows.append(QJsonArray{cell(scan.value("name"), "accent"), cell(scan.value("description"))});
    }
    card.insert("table", table({"Scan", "Description"}, rows));
}

void scanHelpCard(Card &card, const QJsonObject &data)
{
    card.insert("title", "Scan · " + str(orElse(data.value("name"), QString())));
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("arguments"))) {
        const QJsonObject arg = entry.toObject();
        QStringList extra;
        if (arg.contains("default"))
            extra << "default " + text(arg.value("default"));
        if (truthy(arg.value("units")))
            extra << str(arg.value("units"));
        if (truthy(arg.value("constraint")))
            extra << str(arg.value("constraint"));
        QString detail = str(orElse(arg.value("description"), QString()));
        if (detail.endsWith(QLatin1Char('.')))
            detail.chop(1);
        QStringList details;
        for (const QString &part : {detail, extra.join(", ")}) {
            if (!part.isEmpty())
                details << part;
        }
        const bool required = truthy(arg.value("required"));
        rows.append(QJsonArray{cell(arg.value("name"), required ? "accent" : ""), cell(arg.value("type")),
                               cell(required ? "required" : "optional", required ? "info" : ""), cell(details.join(" · "))});
    }
    card.insert("table", table({"Argument", "Type", "", "Details"}, rows));
    if (truthy(data.value("example")))
        card.insert("code", QJsonObject{{"label", "Example call"}, {"text", str(data.value("example"))}});
    if (truthy(data.value("repeat")))
        append(card, "badges", QJsonObject{{"text", str(data.value("repeat"))}, {"tone", "neutral"}});
}

void logsCard(Card &card, const QJsonObject &data)
{
    const QJsonObject levels = object(data.value("levels"));
    for (auto it = levels.begin(); it != levels.end(); ++it)
        append(card, "stats", QJsonObject{{"label", it.key().toLower()}, {"value", str(it.value())}, {"tone", levelTone.value(it.key())}});
    const QJsonArray entries = array(data.value("entries"));
    for (qsizetype index = 0; index < std::min<qsizetype>(entries.size(), 40); ++index) {
        const QJsonObject entry = entries.at(index).toObject();
        const double count = toNumber(orElse(entry.value("count"), 1));
        const QString repeat = count > 1 ? QStringLiteral(" ×") + str(entry.value("count")) : QString();
        append(card, "lines", QJsonObject{
            {"text", str(orElse(entry.value("message"), QString())).left(400)},
            {"tone", levelTone.value(str(entry.value("level")), QStringLiteral("neutral"))},
            {"meta", str(orElse(entry.value("service"), QString())) + " · " + str(orElse(entry.value("level"), QString())) + repeat
                         + " · " + str(orElse(entry.value("last"), QString())).mid(11, 8)}});
    }
    for (const QJsonValue &alarm : array(data.value("alarms")))
        append(card, "findings", QJsonObject{{"severity", "error"}, {"tone", "error"}, {"text", text(alarm).left(400)}, {"next", ""}});
}

void diagnoseCard(Card &card, const QJsonObject &data)
{
    if (data.value("services").isObject()) {
        const QJsonObject services = data.value("services").toObject();
        if (!services.isEmpty()) {
            qsizetype up = 0;
            for (const QJsonValue &status : services)
                up += toneFor(status) == QStringLiteral("ok") ? 1 : 0;
            append(card, "stats", QJsonObject{{"label", "services up"}, {"value", QStringLiteral("%1/%2").arg(up).arg(services.size())},
                                              {"tone", up == services.size() ? "ok" : "warn"}});
        }
    }
    const QJsonObject devices = object(data.value("devices"));
    if (devices.contains("configured")) {
        append(card, "stats", QJsonObject{{"label", "devices"}, {"value", str(devices.value("configured"))}});
        if (truthy(devices.value("disabled")))
            append(card, "stats", QJsonObject{{"label", "disabled"}, {"value", str(devices.value("disabled"))}, {"tone", "warn"}});
    }
    const QJsonObject queues = object(data.value("queues"));
    for (auto it = queues.begin(); it != queues.end(); ++it) {
        const QJsonValue status = object(it.value()).value("status");
        append(card, "stats", QJsonObject{{"label", it.key() + " queue"}, {"value", str(orElse(status, QString()))}, {"tone", toneFor(status)}});
    }
    if (given(data.value("readback"))) {
        const QJsonArray limits = array(data.value("limits"));
        QJsonObject reading{{"value", data.value("readback")}};
        if (!data.value("readback_age_s").isUndefined())
            reading.insert("age_s", data.value("readback_age_s"));
        QJsonObject device{{"name", data.value("device")},
                           {"readback", QJsonObject{{str(data.value("device")), reading}}},
                           {"limits", QJsonObject{{"low", limits.at(0)}, {"high", limits.at(1)}}},
                           {"enabled", data.contains("enabled") ? data.value("enabled") : QJsonValue(true)}};
        deviceReadCard(card, device);
        card.remove("table");
    }
}

void docsCard(Card &card, const QJsonObject &data)
{
    for (const QJsonValue &entry : array(data.value("results"))) {
        const QJsonObject result = entry.toObject();
        append(card, "links", QJsonObject{
            {"title", str(orElse(result.value("title"), orElse(result.value("path"), QString())))},
            {"subtitle", str(orElse(result.value("heading"), QString()))},
            {"url", str(orElse(result.value("url"), QString()))},
            {"snippet", str(orElse(result.value("snippet"), QString())).left(240)}});
    }
}

void statesCard(Card &card, const QJsonObject &data)
{
    const QJsonObject interlock = object(data.value("interlock"));
    if (!interlock.isEmpty()) {
        const bool enabled = truthy(interlock.value("enabled"));
        append(card, "badges", QJsonObject{{"text", enabled ? "interlock on" : "interlock off"}, {"tone", enabled ? "info" : "neutral"}});
        if (truthy(interlock.value("primary_queue_locked")))
            append(card, "badges", QJsonObject{{"text", "queue locked"}, {"tone", "warn"}});
    }
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("states"))) {
        const QJsonObject state = entry.toObject();
        rows.append(QJsonArray{cell(state.value("name")),
                               cell(state.contains("status") ? state.value("status") : state.value("state"), toneFor(state.value("status")))});
    }
    if (!rows.isEmpty())
        card.insert("table", table({"State", "Status"}, rows));
}

QString clock(const QJsonValue &iso)
{
    return str(orElse(iso, QString())).mid(11, 8);
}

QString where(const QJsonObject &origin)
{
    return str(orElse(origin.value("path"), origin.value("file"))) + ":" + str(origin.value("line")) + " ("
        + str(origin.value("function")) + ")";
}

QJsonObject logLine(const QJsonValue &entry)
{
    const QJsonObject record = entry.toObject();
    const QJsonValue tb = record.value("traceback");
    const QString level = str(orElse(record.value("level"), QString()));
    QJsonObject line{{"time", clock(record.value("time"))},
                     {"service", str(orElse(record.value("service"), QString()))},
                     {"level", level},
                     {"tone", levelTone.value(level.toUpper(), QStringLiteral("neutral"))},
                     {"message", str(orElse(record.value("message"), QString()))}};
    if (record.value("id").isDouble())
        line.insert("id", record.value("id"));
    if (truthy(record.value("source")))
        line.insert("source", str(record.value("source")));
    if (truthy(tb)) {
        const QJsonObject traceback = tb.toObject();
        const QJsonValue origin = traceback.value("origin");
        line.insert("traceback", QJsonObject{
            {"exception", str(orElse(traceback.value("root_cause"), orElse(traceback.value("exception"), QString())))},
            {"origin", truthy(origin) ? where(origin.toObject()) : QString()}});
    }
    return line;
}

QJsonObject incident(const QJsonValue &entry)
{
    const QJsonObject raw = entry.toObject();
    const QJsonValue tb = raw.value("traceback");
    const QJsonObject traceback = tb.toObject();
    const QJsonValue origin = traceback.value("origin");
    const QJsonValue scan = raw.value("scan");
    const QJsonArray ids = array(raw.value("ids"));
    const QString severity = str(raw.value("severity"));
    QString scanText;
    if (truthy(scan)) {
        QStringList parts;
        const QJsonObject s = scan.toObject();
        if (truthy(s.value("name")))
            parts << str(s.value("name"));
        if (s.contains("scan_number"))
            parts << "#" + str(s.value("scan_number"));
        scanText = parts.join(" ");
    }
    QJsonArray context;
    for (const QJsonValue &record : array(raw.value("before")))
        context.append(logLine(record));
    for (const QJsonValue &record : array(raw.value("after")))
        context.append(logLine(record));
    QJsonObject item{
        {"ref", str(orElse(raw.value("ref"), QString()))},
        {"title", str(orElse(raw.value("title"), QString()))},
        {"kind", str(orElse(raw.value("kind"), QString()))},
        {"tone", severityTone.value(severity, severity == QStringLiteral("critical") ? "error" : "neutral")},
        {"count", toNumber(orElse(raw.value("count"), 1))},
        {"first", str(orElse(raw.value("first"), QString()))},
        {"last", str(orElse(raw.value("last"), QString()))},
        {"services", orElse(raw.value("services"), QJsonArray())},
        {"devices", orElse(raw.value("devices"), QJsonArray())},
        {"scan", scanText},
        {"related", orElse(raw.value("related"), QJsonArray())},
        {"message", str(orElse(raw.value("message"), QString()))},
        {"likelyCause", str(orElse(raw.value("likely_cause"), QString()))},
        {"suggestion", str(orElse(raw.value("suggestion"), QString()))},
        {"rootCause", truthy(tb) ? str(orElse(traceback.value("root_cause"), QString())) : QString()},
        {"context", context}};
    if (truthy(origin))
        item.insert("origin", QJsonObject{{"where", where(origin.toObject())},
                                          {"code", str(orElse(origin.toObject().value("code"), QString()))}});
    if (raw.value("first_id").isDouble())
        item.insert("firstId", raw.value("first_id"));
    if (raw.value("last_id").isDouble())
        item.insert("lastId", raw.value("last_id"));
    if (truthy(tb))
        item.insert("tracebackRecord", ids.isEmpty() ? raw.value("last_id") : ids.last());
    return item;
}

void logAnalysisCard(Card &card, const QJsonObject &data)
{
    const QJsonObject cache = object(data.value("cache"));
    if (!data.value("incidents_found").isUndefined())
        append(card, "stats", QJsonObject{{"label", "incidents"}, {"value", str(data.value("incidents_found"))},
                                          {"tone", truthy(data.value("incidents_found")) ? "warn" : "ok"}});
    if (!cache.value("records").isUndefined())
        append(card, "stats", QJsonObject{{"label", "records cached"},
                                          {"value", str(cache.value("records"))
                                                        + (truthy(cache.value("capacity")) ? " / " + str(cache.value("capacity")) : QString())}});
    QJsonArray incidents;
    for (const QJsonValue &raw : array(data.value("incidents")))
        incidents.append(incident(raw));
    card.insert("incidents", incidents);
}

void logRangeCard(Card &card, const QJsonObject &data)
{
    QJsonArray records;
    for (const QJsonValue &record : array(data.value("records")))
        records.append(logLine(record));
    card.insert("records", records);
    const QJsonObject selection = object(data.value("selection"));
    if (!selection.value("first_id").isUndefined())
        append(card, "badges", QJsonObject{{"text", "records " + str(selection.value("first_id")) + "–" + str(selection.value("last_id"))},
                                           {"tone", "neutral"}});
}

void codeTracebackCard(Card &card, const QJsonObject &data)
{
    QJsonArray exceptions;
    for (const QJsonValue &entry : array(data.value("exceptions"))) {
        const QJsonObject e = entry.toObject();
        exceptions.append(QJsonObject{{"text", str(orElse(e.value("exception"), QString()))},
                                      {"relation", str(orElse(e.value("relation"), QString()))}});
    }
    card.insert("exceptions", exceptions);
    QJsonArray frames;
    for (const QJsonValue &entry : array(data.value("frames"))) {
        const QJsonObject frame = entry.toObject();
        frames.append(QJsonObject{{"index", toNumber(orElse(frame.value("frame"), 0))},
                                  {"where", str(frame.value("file")) + ":" + str(frame.value("line"))},
                                  {"function", str(orElse(frame.value("in"), orElse(frame.value("function"), QString())))},
                                  {"code", str(orElse(frame.value("code"), QString()))},
                                  {"underRoots", frame.value("under_roots") != QJsonValue(false)},
                                  {"sourceCheck", str(orElse(frame.value("source_check"), QString()))},
                                  {"exceptionIndex", toNumber(orElse(frame.value("exception_index"), 0))}});
    }
    card.insert("frames", frames);
    const QJsonObject record = object(data.value("record"));
    if (truthy(record.value("service")))
        append(card, "badges", QJsonObject{{"text", str(record.value("service")) + " · record " + str(record.value("id"))},
                                           {"tone", "neutral"}});
}

void codeReadCard(Card &card, const QJsonObject &data)
{
    const QJsonArray range = array(data.value("range"));
    QStringList parts;
    for (const QJsonValue &part : {data.value("file"), object(data.value("symbol")).value("name")}) {
        if (truthy(part))
            parts << str(part);
    }
    if (range.size() == 2)
        parts << "lines " + str(range.at(0)) + "–" + str(range.at(1));
    if (truthy(data.value("code")))
        card.insert("code", QJsonObject{{"label", parts.join(" · ")}, {"text", str(data.value("code"))}});
}

void codeSearchCard(Card &card, const QJsonObject &data)
{
    QJsonArray rows;
    for (const QJsonValue &entry : array(data.value("matches"))) {
        const QJsonObject m = entry.toObject();
        rows.append(QJsonArray{cell(str(m.value("file")) + ":" + str(m.value("line")), "accent"),
                               cell(orElse(m.value("in"), QString())), cell(m.value("text"))});
    }
    card.insert("table", table({"Where", "In", "Line"}, rows));
}

using Builder = void (*)(Card &, const QJsonObject &);
const QHash<QString, Builder> builders{
    {"bec_log_analysis", logAnalysisCard}, {"bec_log_range", logRangeCard}, {"bec_code_traceback", codeTracebackCard},
    {"bec_code_read", codeReadCard}, {"bec_code_search", codeSearchCard}, {"bec_status", statusCard},
    {"bec_ping", pingCard}, {"bec_devices", devicesCard}, {"bec_device_info", deviceInfoCard},
    {"bec_device_read", deviceReadCard}, {"bec_queue", queueCard}, {"bec_scan_history", historyCard},
    {"bec_scans", scansCard}, {"bec_scan_help", scanHelpCard}, {"bec_logs", logsCard},
    {"bec_diagnose", diagnoseCard}, {"bec_device_diagnose", diagnoseCard}, {"bec_docs_search", docsCard},
    {"bec_beamline_states", statesCard}};

// --- scan calls ----------------------------------------------------------------------------------

QString pyRepr(const QJsonValue &value, const QStringList &devices)
{
    switch (value.type()) {
    case QJsonValue::String: {
        QString text = value.toString();
        if (devices.contains(text))
            return "dev." + text;
        text.replace(QLatin1Char('\\'), QStringLiteral("\\\\")).replace(QLatin1Char('\''), QStringLiteral("\\'"));
        return "'" + text + "'";
    }
    case QJsonValue::Null:
    case QJsonValue::Undefined:
        return QStringLiteral("None");
    case QJsonValue::Bool:
        return value.toBool() ? QStringLiteral("True") : QStringLiteral("False");
    case QJsonValue::Double:
        return number(value.toDouble());
    case QJsonValue::Array:
        return "[" + join(value.toArray(), ", ", [&](const QJsonValue &item) { return pyRepr(item, devices); }) + "]";
    case QJsonValue::Object: {
        QStringList entries;
        const QJsonObject object = value.toObject();
        for (auto it = object.begin(); it != object.end(); ++it)
            entries << "'" + it.key() + "': " + pyRepr(it.value(), {});
        return "{" + entries.join(", ") + "}";
    }
    }
    return {};
}

// the value itself, or the JSON a string holds
QJsonValue parseMaybeJson(const QJsonValue &value)
{
    if (!value.isString())
        return value;
    const QJsonDocument document = QJsonDocument::fromJson(value.toString().toUtf8());
    if (document.isArray())
        return document.array();
    if (document.isObject())
        return document.object();
    return value;
}

QJsonValue fromVariant(const QVariant &value)
{
    if (value.metaType() == QMetaType::fromType<QJSValue>())
        return QJsonValue::fromVariant(value.value<QJSValue>().toVariant());
    return QJsonValue::fromVariant(value);
}

} // namespace

namespace BecCards {

QJsonObject parseToolResult(const QJsonValue &output)
{
    QJsonValue value = output;
    if (value.isString()) {
        const QString trimmed = value.toString().trimmed();
        if (!trimmed.startsWith(QLatin1Char('{')))
            return {};
        value = QJsonDocument::fromJson(trimmed.toUtf8()).object();
    }
    const QJsonObject result = value.toObject();
    return result.value("summary").isString() ? result : QJsonObject();
}

QJsonObject buildCard(const QString &tool, const QJsonObject &answer)
{
    const QJsonArray list = findings(answer);
    const QJsonValue ok = answer.value("ok");
    Card card = emptyCard(tool, titles.value(tool, underscoresToSpaces(tool)), str(orElse(answer.value("summary"), QString())),
                          overallTone(list, ok.isUndefined() || truthy(ok)));
    card.insert("findings", list);
    // a shape this version does not know shows only the summary and findings
    if (const Builder builder = builders.value(tool); builder && answer.value("data").isObject())
        builder(card, answer.value("data").toObject());
    if (truthy(answer.value("truncated")))
        append(card, "badges", QJsonObject{{"text", "truncated"}, {"tone", "warn"}});
    return card;
}

QJsonObject scanResultCard(const QJsonObject &result, const QString &callText)
{
    QString tone;
    QString summary;
    const bool submitted = truthy(result.value("submitted"));
    const bool rejected = result.value("accepted") == QJsonValue(false);
    if (submitted && !rejected) {
        tone = QStringLiteral("ok");
        summary = QStringLiteral("Scan submitted to the queue.");
        const QJsonValue status = result.value("status");
        const QJsonValue number = result.value("scan_number");
        if (status == QJsonValue("running") && given(number))
            summary = "Scan #" + str(number) + " is running.";
        else if (given(number))
            summary = "Scan #" + str(number) + " is in the queue.";
        else if (status == QJsonValue("queued"))
            summary = QStringLiteral("Scan accepted and queued; it gets its number when it starts.");
        else if (status == QJsonValue("undecided")) {
            tone = QStringLiteral("warn");
            summary = QStringLiteral("Submitted, but the scan server has not answered yet.");
        } else if (truthy(status))
            summary += " Status: " + str(status) + ".";
    } else if (submitted) {
        tone = QStringLiteral("error");
        summary = ("The scan server rejected the scan: " + str(orElse(result.value("guard_message"), QString()))).trimmed();
    } else {
        tone = QStringLiteral("error");
        summary = str(orElse(result.value("error"), QStringLiteral("The scan was not submitted.")));
    }
    Card card = emptyCard("bec_run_scan", "Scan", summary, tone);
    card.insert("code", QJsonObject{{"label", "Call"}, {"text", callText}});
    for (const char *key : {"scan_number", "scan_id", "request_id"}) {
        const QJsonValue value = result.value(QLatin1String(key));
        if (given(value))
            append(card, "stats", QJsonObject{{"label", underscoresToSpaces(QString::fromLatin1(key))}, {"value", str(value).left(13)}});
    }
    return card;
}

QString title(const QString &tool)
{
    return titles.value(tool);
}

QString activity(const QString &tool)
{
    return activities.value(tool);
}

QJsonObject toolView(const QString &tool, const QString &status, const QJsonValue &input, const QJsonValue &output)
{
    QJsonObject view;
    const auto label = [&view](const QString &text) {
        if (!text.isEmpty())
            view.insert("label", text);
    };
    if (status == QStringLiteral("pending") || status == QStringLiteral("running")) {
        label(activity(tool).isEmpty() ? title(tool) : activity(tool));
        return view;
    }
    label(title(tool));
    if (status == QStringLiteral("error"))
        return view;
    const QJsonObject result = parseToolResult(output);
    if (tool == QStringLiteral("bec_run_scan")) {
        const QString callText = formatScanCall(input.toObject());
        // the submission's outcome: in a tool_result()'s data, or the output itself
        QJsonValue raw = result.value("data");
        if (!truthy(raw))
            raw = output.isString() ? QJsonValue(QJsonDocument::fromJson(output.toString().toUtf8()).object()) : output;
        if (raw.toObject().isEmpty()) {
            view.insert("summary", callText);
            return view;
        }
        const QJsonObject scan = scanResultCard(raw.toObject(), callText);
        view.insert("summary", scan.value("summary"));
        view.insert("tone", scan.value("tone"));
        view.insert("card", scan);
        return view;
    }
    if (result.isEmpty())
        return view;
    // tools the cards do not know still share the result shape: summary and findings show
    const QJsonObject card = buildCard(tool, result);
    view.insert("summary", card.value("summary"));
    view.insert("tone", card.value("tone"));
    view.insert("card", card);
    return view;
}

QString formatScanCall(const QJsonObject &input)
{
    static const QRegularExpression deviceName(QStringLiteral("^[A-Za-z_][A-Za-z0-9_]*$"));
    const QJsonValue name = input.contains("scan_name") ? input.value("scan_name")
        : input.contains("scan")                        ? input.value("scan")
                                                        : QJsonValue(QStringLiteral("?"));
    const QJsonArray args = parseMaybeJson(input.value("args")).toArray();
    const QJsonObject kwargs = parseMaybeJson(input.value("kwargs")).toObject();
    QStringList devices;
    for (const QJsonValue &arg : args) {
        if (arg.isString() && deviceName.match(arg.toString()).hasMatch())
            devices << arg.toString();
    }
    QStringList parts;
    for (const QJsonValue &arg : args)
        parts << pyRepr(arg, devices);
    for (auto it = kwargs.begin(); it != kwargs.end(); ++it)
        parts << it.key() + "=" + pyRepr(it.value(), {});
    return "scans." + str(name) + "(" + parts.join(", ") + ")";
}

} // namespace BecCards

QVariant BecCardsQml::parseToolResult(const QVariant &output) const
{
    const QJsonObject result = BecCards::parseToolResult(fromVariant(output));
    return result.isEmpty() ? QVariant() : QVariant(result.toVariantMap());
}

QVariantMap BecCardsQml::buildCard(const QString &tool, const QVariant &answer) const
{
    return BecCards::buildCard(tool, fromVariant(answer).toObject()).toVariantMap();
}

QVariantMap BecCardsQml::scanResultCard(const QVariant &result, const QString &callText) const
{
    return BecCards::scanResultCard(fromVariant(result).toObject(), callText).toVariantMap();
}

QVariant BecCardsQml::title(const QString &tool) const
{
    const QString text = BecCards::title(tool);
    return text.isEmpty() ? QVariant() : QVariant(text);
}

QVariant BecCardsQml::activity(const QString &tool) const
{
    const QString text = BecCards::activity(tool);
    return text.isEmpty() ? QVariant() : QVariant(text);
}

QString BecCardsQml::formatScanCall(const QVariant &input) const
{
    return BecCards::formatScanCall(fromVariant(input).toObject());
}

QVariantMap BecCardsQml::toolView(const QString &tool, const QString &status, const QVariant &input, const QVariant &output) const
{
    return BecCards::toolView(tool, status, fromVariant(input), fromVariant(output)).toVariantMap();
}
