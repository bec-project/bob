#include "bobsession.h"

#include "bobpaths.h"

#include <QCoreApplication>
#include <QFileInfo>
#include <QJsonDocument>
#include <QLocalSocket>
#include <QProcess>

#include <memory>

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
        map.insert(QStringLiteral("container"), container);
        map.insert(QStringLiteral("config"), config);
        map.insert(QStringLiteral("runningConfig"), runningConfig);
        break;
    case Lost:
        map.insert(QStringLiteral("reason"), reason);
        map.insert(QStringLiteral("config"), config);
        map.insert(QStringLiteral("container"), container);
        break;
    }
    return map;
}

bool BobStatus::operator==(const BobStatus &other) const
{
    return state == other.state && error == other.error && config == other.config
        && connection == other.connection && container == other.container
        && runningConfig == other.runningConfig && reason == other.reason;
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
}

void BobSession::setStatus(const BobStatus &status)
{
    m_status = status;
    emit statusChanged();
}

void BobSession::join(const QString &binary, const QString &config)
{
    if (m_status.state == BobStatus::Joined || m_status.state == BobStatus::Joining)
        return;
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

    // bob resolves relative paths beside its bob.toml
    auto *process = new QProcess(this);
    m_process = process;
    process->setProgram(binary);
    process->setArguments({QStringLiteral("--config"), config});
    process->setWorkingDirectory(configFile.absolutePath());
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

void BobSession::dropLease()
{
    m_buffer.clear();
    m_greeted = false;
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
            connectBroker(generation, socket);
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
        if (!m_greeted)
            handleReply(generation, line);
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
    const QString error = reply.value(QStringLiteral("error")).toString();
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
    joined.container = joined.connection.value(QStringLiteral("container")).toString();
    joined.config = m_status.config;
    joined.runningConfig = reply.value(QStringLiteral("config")).toString(m_status.config);
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
    dropLease();
    BobStatus lost;
    lost.state = BobStatus::Lost;
    lost.reason = error.isEmpty() ? QStringLiteral("bob stopped: its broker ended this app's lease")
                                  : QStringLiteral("lost the connection to bob's broker: %1").arg(error);
    lost.config = joined.config;
    lost.container = joined.container;
    setStatus(lost);
}
