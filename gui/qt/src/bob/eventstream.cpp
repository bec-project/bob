#include "eventstream.h"

#include "opencodeapi.h"

#include <QJsonDocument>
#include <QNetworkReply>

QStringList SseParser::push(const QByteArray &chunk)
{
    m_buffer += chunk;
    QStringList out;
    qsizetype newline;
    while ((newline = m_buffer.indexOf('\n')) != -1) {
        QByteArray line = m_buffer.left(newline);
        m_buffer.remove(0, newline + 1);
        if (line.endsWith('\r'))
            line.chop(1);
        if (line.isEmpty()) {
            if (!m_data.isEmpty())
                out << m_data.join(QLatin1Char('\n'));
            m_data.clear();
        } else if (line.startsWith("data:")) {
            m_data << QString::fromUtf8(line.mid(line.size() > 5 && line[5] == ' ' ? 6 : 5));
        }
    }
    return out;
}

EventStream::EventStream(OpencodeApi *api, QObject *parent)
    : QObject(parent)
    , m_api(api)
{
    m_heartbeat.setInterval(45000);
    m_heartbeat.setSingleShot(true);
    connect(&m_heartbeat, &QTimer::timeout, this, [this] {
        if (m_reply)
            m_reply->abort();
    });
    m_retry.setSingleShot(true);
    connect(&m_retry, &QTimer::timeout, this, &EventStream::connectNow);
}

EventStream::~EventStream()
{
    stop();
}

void EventStream::start()
{
    m_stopped = false;
    m_opened = false;
    m_delay = 500;
    connectNow();
}

void EventStream::stop()
{
    const bool wasRunning = !m_stopped;
    m_stopped = true;
    m_heartbeat.stop();
    m_retry.stop();
    if (m_reply) {
        QNetworkReply *reply = m_reply;
        m_reply = nullptr;
        reply->disconnect(this);
        reply->abort();
        reply->deleteLater();
    }
    if (wasRunning)
        emit stateChanged(QStringLiteral("closed"), {});
}

void EventStream::connectNow()
{
    if (m_stopped || !m_api)
        return;
    emit stateChanged(m_opened ? QStringLiteral("retrying") : QStringLiteral("connecting"), {});
    QNetworkRequest request = m_api->newRequest(QStringLiteral("/event"));
    request.setRawHeader("Accept", "text/event-stream");
    request.setAttribute(QNetworkRequest::CacheLoadControlAttribute, QNetworkRequest::AlwaysNetwork);
    m_parser = SseParser();
    m_replyOpen = false;
    QNetworkReply *reply = m_api->network()->get(request);
    m_reply = reply;
    m_heartbeat.start();
    connect(reply, &QNetworkReply::metaDataChanged, this, [this, reply] {
        if (reply != m_reply || m_replyOpen)
            return;
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        if (status == 401) {
            m_stopped = true;
            m_heartbeat.stop();
            m_reply = nullptr;
            reply->disconnect(this);
            reply->abort();
            reply->deleteLater();
            emit unauthorized();
            emit stateChanged(QStringLiteral("closed"), {});
            return;
        }
        if (status < 200 || status >= 300)
            return;  // finished() reports it
        m_replyOpen = true;
        const bool reconnected = m_opened;
        m_opened = true;
        m_delay = 500;
        emit opened(reconnected);
        emit stateChanged(QStringLiteral("open"), {});
    });
    connect(reply, &QNetworkReply::readyRead, this, [this, reply] {
        if (reply != m_reply)
            return;
        m_heartbeat.start();
        for (const QString &data : m_parser.push(reply->readAll())) {
            const QJsonDocument document = QJsonDocument::fromJson(data.toUtf8());
            if (document.isObject())
                emit event(document.object());
            else
                qWarning("bad event: %s", qPrintable(data.left(200)));
            if (reply != m_reply)
                return;
        }
    });
    connect(reply, &QNetworkReply::finished, this, [this, reply] {
        if (reply != m_reply)
            return;
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        QString detail = reply->error() == QNetworkReply::NoError ? QStringLiteral("the event stream ended")
                                                                  : reply->errorString();
        if (status != 0 && (status < 200 || status >= 300))
            detail = QStringLiteral("event stream failed (%1)").arg(status);
        m_reply = nullptr;
        reply->deleteLater();
        ended(detail);
    });
}

void EventStream::ended(const QString &detail)
{
    m_heartbeat.stop();
    if (m_stopped)
        return;
    emit stateChanged(QStringLiteral("retrying"), detail);
    m_retry.start(m_delay);
    m_delay = qMin(m_delay * 2, 10000);
}
