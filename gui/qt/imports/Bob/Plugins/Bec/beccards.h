#pragma once

#include <QJsonObject>
#include <QJsonValue>
#include <QObject>
#include <QQmlEngine>
#include <QVariant>

// BEC tool results turned into report cards (a port of bec_mcp_chat's cards.py). Every card has
// the same shape, so one view (BecCard.qml) draws all of them:
//
//   {kind, title, summary, tone, badges: [{text, tone}], stats: [{label, value, tone}],
//    gauge: {label, value, age, low, high, fraction, tone}, table: {columns, rows: [[{text, tone}]], note},
//    findings: [{severity, tone, text, next}], code: {label, text}, links: [{title, subtitle, url, snippet}],
//    lines: [{text, tone, meta}], incidents: [...], records: [...], frames: [...], exceptions: [...]}
//
// Tones: neutral, ok, info, warn, error, accent. Objects keep Qt's key order (alphabetical), not
// the order the server wrote them in.
namespace BecCards {
// The tool_result() object in a tool's output (its text, or its structured value); empty if it is
// not one.
QJsonObject parseToolResult(const QJsonValue &output);
// A card for one tool result.
QJsonObject buildCard(const QString &tool, const QJsonObject &answer);
// The card after a scan submission through bec_run_scan.
QJsonObject scanResultCard(const QJsonObject &result, const QString &callText);
// What a tool is called, and what its row says while it runs; empty for a tool the cards do not know.
QString title(const QString &tool);
QString activity(const QString &tool);
// How a call of one of the server's tools is drawn: {label, summary, tone, card}, each left out
// where the app's own row says it better. `tool` is the name without the server's prefix.
QJsonObject toolView(const QString &tool, const QString &status, const QJsonValue &input, const QJsonValue &output);
// bec_run_scan's arguments ({scan | scan_name, args, kwargs}) as a scientist would type the call:
// scans.line_scan(dev.samx, -1, 1, steps=10). String positional arguments that look like names
// are taken to be devices, as scan arguments almost always are.
QString formatScanCall(const QJsonObject &input);
}

// The cards and scan calls for the plugin's QML views.
class BecCardsQml : public QObject
{
    Q_OBJECT
    QML_NAMED_ELEMENT(BecCards)
    QML_SINGLETON

public:
    using QObject::QObject;

    // The parsed tool result, or undefined when the output is not one.
    Q_INVOKABLE QVariant parseToolResult(const QVariant &output) const;
    Q_INVOKABLE QVariantMap buildCard(const QString &tool, const QVariant &answer) const;
    Q_INVOKABLE QVariantMap scanResultCard(const QVariant &result, const QString &callText) const;
    // undefined for a tool the cards do not know
    Q_INVOKABLE QVariant title(const QString &tool) const;
    Q_INVOKABLE QVariant activity(const QString &tool) const;
    Q_INVOKABLE QString formatScanCall(const QVariant &input) const;
    Q_INVOKABLE QVariantMap toolView(const QString &tool, const QString &status, const QVariant &input,
                                     const QVariant &output) const;
};
