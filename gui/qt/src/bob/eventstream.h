#pragma once

#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QTimer>

class OpencodeApi;
class QNetworkReply;

// Splits a text/event-stream body into the payloads of its data fields.
class SseParser
{
public:
    QStringList push(const QByteArray &chunk);

private:
    QByteArray m_buffer;
    QStringList m_data;
};

// Follows OpenCode's GET /event. It reconnects with backoff, and treats a silent connection (no
// heartbeat) as lost.
class EventStream : public QObject
{
    Q_OBJECT
public:
    // connecting, open, retrying or closed
    explicit EventStream(OpencodeApi *api, QObject *parent = nullptr);
    ~EventStream() override;

    void start();
    void stop();
    void setHeartbeatTimeout(int milliseconds) { m_heartbeat.setInterval(milliseconds); }

signals:
    void event(const QJsonObject &event);
    // The stream (re)opened: anything sent while it was down has to be fetched again.
    void opened(bool reconnected);
    void stateChanged(const QString &state, const QString &detail);
    // The server refused the password (bob restarted with a new one): the stream stops.
    void unauthorized();

private:
    void connectNow();
    void ended(const QString &detail);

    QPointer<OpencodeApi> m_api;
    QPointer<QNetworkReply> m_reply;
    SseParser m_parser;
    QTimer m_heartbeat;
    QTimer m_retry;
    int m_delay = 500;
    bool m_opened = false;
    bool m_stopped = true;
    bool m_replyOpen = false;
};
