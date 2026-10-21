#include "bobsession.h"

#include "bobpaths.h"

#include <QCoreApplication>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QLocalSocket>
#include <QProcess>

#include <memory>

#include <sys/stat.h>

namespace {

QString tail(const QByteArray &text, int lines)
{
    QStringList all = QString::fromUtf8(text).split(QLatin1Char('\n'));
    while (!all.isEmpty() && all.last().isEmpty())
        all.removeLast();
    return all.mid(qMax(0, all.size() - lines)).join(QLatin1Char('\n'));
}

}  // namespace

BobStatus BobStatus::idle(const QString &error)
{
    BobStatus status;
    status.error = error;
    return status;
}

QString BobStatus::stateName() const
{
    switch (state) {
    case Idle:
        return QStringLiteral("idle");
    case Joining:
        return QStringLiteral("joining");
    case Joined:
        return QStringLiteral("joined");
    case Lost:
        return QStringLiteral("lost");
    }
    return {};
}

QVariantMap BobStatus::toVariant() const
{
    QVariantMap map{{QStringLiteral("state"), stateName()}};
    switch (state) {
    case Idle:
        map.insert(QStringLiteral("error"), error.isEmpty() ? QVariant() : QVariant(error));
        break;
    case Joining:
        map.insert(QStringLiteral("config"), config);
        break;
    case Joined:
        map.insert(QStringLiteral("connection"), connection.toVariantMap());
        map.insert(QStringLiteral("name"), name);
        map.insert(QStringLiteral("config"), config);
        map.insert(QStringLiteral("runningConfig"), runningConfig);
        map.insert(QStringLiteral("consumers"), consumers);
        break;
    case Lost:
        map.insert(QStringLiteral("reason"), reason);
        map.insert(QStringLiteral("config"), config);
        map.insert(QStringLiteral("name"), name);
        break;
    }
    return map;
}

bool BobStatus::operator==(const BobStatus &other) const
{
    return state == other.state && error == other.error && config == other.config
        && connection == other.connection && name == other.name
        && runningConfig == other.runningConfig && consumers == other.consumers && reason == other.reason;
}

BobSession::BobSession(QObject *parent)
    : QObject(parent)
{
    m_helloTimer.setSingleShot(true);
    connect(&m_helloTimer, &QTimer::timeout, this, [this] {
        fail(m_generation, QStringLiteral("bob's broker did not answer within %1 s").arg(m_helloTimeout / 1000));
    });
}

BobSession::~BobSession()
{
    dropProcess();
    dropLease();
    dropStop();
}

void BobSession::setStatus(const BobStatus &status)
{
    m_status = status;
    emit statusChanged();
}

void BobSession::join(const QString &binary, const QString &config)
{
    if (m_status.state == BobStatus::Joined) {
        // holding bob already: keep it, but say which bob.toml was asked for, so a different one
        // shows up as a mismatch the user can act on
        if (m_status.config != config) {
            BobStatus status = m_status;
            status.config = config;
            setStatus(status);
        }
        return;
    }
    if (m_status.state == BobStatus::Joining)
        return;
    start(binary, config, QString());
}

void BobSession::restart(const QString &binary, const QString &config, const QString &reason)
{
    leave();
    start(binary, config, reason.isEmpty() ? QStringLiteral("bob was restarted") : reason);
}

void BobSession::start(const QString &binary, const QString &config, const QString &stopReason)
{
    const quint64 generation = ++m_generation;
    BobStatus joining;
    joining.state = BobStatus::Joining;
    joining.config = config;
    setStatus(joining);

    const QFileInfo configFile(config);
    if (config.isEmpty()) {
        fail(generation, QStringLiteral("Choose bob's configuration file (bob.toml) first."));
        return;
    }
    if (!configFile.isFile()) {
        fail(generation, QStringLiteral("%1 is not a file").arg(config));
        return;
    }
    if (binary.trimmed().isEmpty()) {
        fail(generation, QStringLiteral("No bob executable: choose one, or put bob-manager on the PATH."));
        return;
    }
    if (!BobPaths::isExecutable(binary)) {
        fail(generation, QStringLiteral("%1 is not an executable").arg(binary));
        return;
    }
    m_binary = binary;
    m_stopReason = stopReason;
    locate(generation);
}

// Runs `bob-manager --config <bob.toml>`, which makes sure the user's broker runs and prints its
// socket.
void BobSession::locate(quint64 generation)
{
    const QString binary = m_binary;
    // bob resolves relative paths beside its bob.toml
    auto *process = new QProcess(this);
    m_process = process;
    process->setProgram(binary);
    process->setArguments({QStringLiteral("--config"), m_status.config});
    process->setWorkingDirectory(QFileInfo(m_status.config).absolutePath());
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    environment.insert(QStringLiteral("PATH"), BobPaths::enginePath());
    process->setProcessEnvironment(environment);
    process->setStandardInputFile(QProcess::nullDevice());
    connect(process, &QProcess::finished, this, [this, generation] { located(generation); });
    connect(process, &QProcess::errorOccurred, this, [this, generation, process, binary](QProcess::ProcessError error) {
        if (error == QProcess::FailedToStart)
            fail(generation, QStringLiteral("cannot run %1: %2").arg(binary, process->errorString()));
    });
    process->start();
}

void BobSession::leave()
{
    ++m_generation;
    m_helloTimer.stop();
    dropProcess();
    dropLease();
    dropStop();
    if (m_status != BobStatus::idle())
        setStatus(BobStatus::idle());
}

void BobSession::fail(quint64 generation, const QString &error)
{
    if (generation != m_generation)
        return;
    m_helloTimer.stop();
    dropProcess();
    dropLease();
    dropStop();
    setStatus(BobStatus::idle(error));
}

void BobSession::dropProcess()
{
    if (!m_process)
        return;
    QProcess *process = m_process;
    m_process = nullptr;
    process->disconnect(this);
    if (process->state() != QProcess::NotRunning) {
        process->kill();
        process->waitForFinished(1000);
    }
    process->deleteLater();
}

void BobSession::dropStop()
{
    m_stopReason.clear();
    m_stopWatch.stop();
    m_stopWatch.disconnect(this);
    if (!m_stopRequest)
        return;
    QLocalSocket *request = m_stopRequest;
    m_stopRequest = nullptr;
    request->disconnect(this);
    request->abort();
    request->deleteLater();
}

void BobSession::dropLease()
{
    m_buffer.clear();
    m_greeted = false;
    m_stoppedReason.clear();
    if (!m_lease)
        return;
    QLocalSocket *lease = m_lease;
    m_lease = nullptr;
    lease->disconnect(this);
    lease->abort();
    lease->deleteLater();
}

void BobSession::located(quint64 generation)
{
    if (generation != m_generation || !m_process)
        return;
    QProcess *process = m_process;
    const QByteArray out = process->readAllStandardOutput();
    const QByteArray err = process->readAllStandardError();
    const bool ok = process->exitStatus() == QProcess::NormalExit && process->exitCode() == 0;
    const QString how = process->exitStatus() == QProcess::CrashExit
        ? QStringLiteral("killed")
        : QStringLiteral("exit code %1").arg(process->exitCode());
    dropProcess();
    if (!ok) {
        fail(generation, QStringLiteral("bob could not start (%1):\n%2").arg(how, tail(err, 20)));
        return;
    }
    for (const QByteArray &line : out.split('\n')) {
        const QJsonObject object = QJsonDocument::fromJson(line.trimmed()).object();
        const QString socket = object.value(QStringLiteral("socket")).toString();
        if (!socket.isEmpty()) {
            if (m_stopReason.isEmpty())
                connectBroker(generation, socket);
            else
                stopBroker(generation, socket);
            return;
        }
    }
    fail(generation, QStringLiteral("bob printed no broker socket:\n%1").arg(tail(out, 20)));
}

void BobSession::connectBroker(quint64 generation, const QString &socketPath)
{
    auto *lease = new QLocalSocket(this);
    m_lease = lease;
    m_buffer.clear();
    m_greeted = false;
    auto connected = std::make_shared<bool>(false);
    connect(lease, &QLocalSocket::connected, this, [this, generation, lease, connected] {
        if (generation != m_generation || lease != m_lease)
            return;
        *connected = true;
        const QJsonObject hello{
            {QStringLiteral("consumer"), QStringLiteral("bob-gui")},
            {QStringLiteral("pid"), QCoreApplication::applicationPid()},
            {QStringLiteral("config"), m_status.config},
        };
        lease->write(QJsonDocument(hello).toJson(QJsonDocument::Compact) + '\n');
        m_helloTimer.start(m_helloTimeout);
    });
    connect(lease, &QLocalSocket::readyRead, this, [this, generation] { leaseData(generation); });
    auto ended = [this, generation, lease, connected, socketPath](const QString &error) {
        if (generation != m_generation || lease != m_lease)
            return;
        if (!*connected) {
            fail(generation, QStringLiteral("cannot reach bob's broker at %1: %2").arg(socketPath, error));
            return;
        }
        leaseEnded(generation, error);
    };
    connect(lease, &QLocalSocket::disconnected, this, [ended] { ended({}); });
    connect(lease, &QLocalSocket::errorOccurred, this, [ended, lease](QLocalSocket::LocalSocketError error) {
        ended(error == QLocalSocket::PeerClosedError ? QString() : lease->errorString());
    });
    lease->connectToServer(socketPath);
}

void BobSession::leaseData(quint64 generation)
{
    if (generation != m_generation || !m_lease)
        return;
    m_buffer += m_lease->readAll();
    qsizetype newline;
    while ((newline = m_buffer.indexOf('\n')) != -1) {
        const QByteArray line = m_buffer.left(newline);
        m_buffer.remove(0, newline + 1);
        if (!m_greeted) {
            handleReply(generation, line);
        } else {
            // the broker says why it ends the lease before it does
            const QString stopped = QJsonDocument::fromJson(line).object().value(QStringLiteral("stopped")).toString();
            if (!stopped.isEmpty())
                m_stoppedReason = stopped;
        }
        if (generation != m_generation)
            return;
    }
}

void BobSession::handleReply(quint64 generation, const QByteArray &line)
{
    m_helloTimer.stop();
    const QJsonDocument document = QJsonDocument::fromJson(line.trimmed());
    if (!document.isObject()) {
        fail(generation, QStringLiteral("bob's broker closed the connection before bob was up"));
        return;
    }
    const QJsonObject reply = document.object();
    // an error, or bob stopping (for everyone) while this client waited for it
    QString error = reply.value(QStringLiteral("error")).toString();
    if (error.isEmpty())
        error = reply.value(QStringLiteral("stopped")).toString();
    if (!error.isEmpty()) {
        fail(generation, error);
        return;
    }
    const QJsonValue connection = reply.value(QStringLiteral("connection"));
    if (!connection.isObject()) {
        fail(generation, QStringLiteral("bob's broker sent no connection"));
        return;
    }
    m_greeted = true;
    BobStatus joined;
    joined.state = BobStatus::Joined;
    joined.connection = connection.toObject();
    joined.name = joined.connection.value(QStringLiteral("name")).toString();
    joined.config = m_status.config;
    joined.runningConfig = reply.value(QStringLiteral("config")).toString(m_status.config);
    joined.consumers = reply.value(QStringLiteral("consumers")).toInt(1);
    setStatus(joined);
}

void BobSession::leaseEnded(quint64 generation, const QString &error)
{
    if (generation != m_generation)
        return;
    if (!m_greeted) {
        fail(generation, error.isEmpty() ? QStringLiteral("bob's broker closed the connection before bob was up")
                                         : QStringLiteral("lost the connection to bob's broker: %1").arg(error));
        return;
    }
    const BobStatus joined = m_status;
    const QString stopped = m_stoppedReason;
    dropLease();
    BobStatus lost;
    lost.state = BobStatus::Lost;
    if (!stopped.isEmpty())
        lost.reason = stopped;
    else if (error.isEmpty())
        lost.reason = QStringLiteral("bob stopped: its broker ended this app's lease");
    else
        lost.reason = QStringLiteral("lost the connection to bob's broker: %1").arg(error);
    lost.config = joined.config;
    lost.name = joined.name;
    setStatus(lost);
}

// Asks the user's broker to stop bob for every client and exit, waits until its socket is gone
// (or replaced by a new broker's), then joins: bob-manager starts a broker with this bob.toml.
void BobSession::stopBroker(quint64 generation, const QString &socket)
{
    const QString reason = m_stopReason;
    m_stopReason.clear();
    auto inode = [socket]() -> qint64 {
        struct stat info;
        return ::stat(QFile::encodeName(socket).constData(), &info) == 0 ? qint64(info.st_ino) : -1;
    };
    const qint64 before = inode();
    auto *request = new QLocalSocket(this);
    m_stopRequest = request;
    m_stopWatch.setInterval(50);
    connect(&m_stopWatch, &QTimer::timeout, this, [this, generation, inode, before] {
        if (generation != m_generation)
            return;
        // the broker removes its socket as soon as it stops listening, before it removes bob
        if (inode() != before) {
            dropStop();
            locate(generation);
        } else if (m_stopWaited.elapsed() > m_stopTimeout) {
            fail(generation, QStringLiteral("bob's broker did not stop"));
        }
    });
    connect(request, &QLocalSocket::connected, this, [this, generation, request, reason] {
        if (generation != m_generation)
            return;
        const QJsonObject stop{
            {QStringLiteral("consumer"), QStringLiteral("bob-gui")},
            {QStringLiteral("pid"), QCoreApplication::applicationPid()},
            {QStringLiteral("stop"), true},
            {QStringLiteral("reason"), reason},
        };
        request->write(QJsonDocument(stop).toJson(QJsonDocument::Compact) + '\n');
        m_stopWaited.start();
        m_stopWatch.start();
    });
    connect(request, &QLocalSocket::errorOccurred, this, [this, generation, request](QLocalSocket::LocalSocketError) {
        if (generation != m_generation || m_stopWatch.isActive())
            return;
        // no broker answers: nothing runs, so start bob
        request->disconnect(this);
        request->deleteLater();
        m_stopRequest = nullptr;
        locate(generation);
    });
    request->connectToServer(socket);
}
