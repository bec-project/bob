#include "runningbobs.h"

#include "bobpaths.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QProcess>
#include <QRegularExpression>

#include <functional>

namespace {

const QString launchLabel = QStringLiteral("io.bob.launch");
const QString opencodePort = QStringLiteral("4096/tcp");

// Runs `engine args` with the widened PATH; `done` gets stdout, or nothing when it failed.
void run(QObject *context, const QString &engine, const QStringList &args,
         std::function<void(std::optional<QByteArray>)> done)
{
    auto *process = new QProcess(context);
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    environment.insert(QStringLiteral("PATH"), BobPaths::enginePath());
    process->setProcessEnvironment(environment);
    process->setStandardInputFile(QProcess::nullDevice());
    process->setStandardErrorFile(QProcess::nullDevice());
    QObject::connect(process, &QProcess::finished, context, [process, done] {
        const bool ok = process->exitStatus() == QProcess::NormalExit && process->exitCode() == 0;
        done(ok ? std::optional<QByteArray>(process->readAllStandardOutput()) : std::nullopt);
        process->deleteLater();
    });
    QObject::connect(process, &QProcess::errorOccurred, context, [process, done](QProcess::ProcessError error) {
        if (error != QProcess::FailedToStart)
            return;
        done(std::nullopt);
        process->deleteLater();
    });
    process->start(engine, args);
}

}  // namespace

QVariantMap RunningBob::toVariant() const
{
    return {
        {QStringLiteral("url"), url},
        {QStringLiteral("password"), password},
        {QStringLiteral("container"), container},
        {QStringLiteral("runtime"), runtime},
        {QStringLiteral("workspace"), workspace},
        {QStringLiteral("image"), image},
        {QStringLiteral("started"), started},
    };
}

std::optional<RunningBob> runningBob(const QJsonObject &entry, const QString &runtime)
{
    const QJsonObject config = entry.value(QStringLiteral("Config")).toObject();
    if (!config.value(QStringLiteral("Labels")).toObject().contains(launchLabel))
        return std::nullopt;
    QString password;
    bool hasPassword = false;
    for (const QJsonValue &value : config.value(QStringLiteral("Env")).toArray()) {
        const QString variable = value.toString();
        if (variable.startsWith(QStringLiteral("OPENCODE_SERVER_PASSWORD="))) {
            password = variable.mid(variable.indexOf(QLatin1Char('=')) + 1);
            hasPassword = true;
            break;
        }
    }
    if (!hasPassword)
        return std::nullopt;
    const QJsonArray bindings = entry.value(QStringLiteral("NetworkSettings"))
                                    .toObject()
                                    .value(QStringLiteral("Ports"))
                                    .toObject()
                                    .value(opencodePort)
                                    .toArray();
    if (bindings.isEmpty())
        return std::nullopt;
    const QJsonObject binding = bindings.first().toObject();
    const QString port = binding.value(QStringLiteral("HostPort")).toString();
    if (port.isEmpty())
        return std::nullopt;
    const QString ip = binding.value(QStringLiteral("HostIp")).toString();
    QString host;
    if (ip.isEmpty() || ip == QStringLiteral("0.0.0.0"))
        host = QStringLiteral("127.0.0.1");
    else if (ip == QStringLiteral("::"))
        host = QStringLiteral("[::1]");
    else if (ip.contains(QLatin1Char(':')))
        host = QStringLiteral("[%1]").arg(ip);
    else
        host = ip;

    RunningBob bob;
    bob.url = QStringLiteral("http://%1:%2").arg(host, port);
    bob.password = password;
    bob.container = entry.value(QStringLiteral("Name")).toString();
    while (bob.container.startsWith(QLatin1Char('/')))
        bob.container.remove(0, 1);
    bob.runtime = runtime;
    bob.workspace = config.value(QStringLiteral("WorkingDir")).toString();
    bob.image = config.value(QStringLiteral("Image")).toString();
    bob.started = entry.value(QStringLiteral("State")).toObject().value(QStringLiteral("StartedAt")).toString();
    return bob;
}

void RunningBobLister::refresh()
{
    const quint64 round = ++m_round;
    m_collecting.clear();
    m_pending = 0;
    QList<QPair<QString, QString>> engines;
    for (const QString &runtime : {QStringLiteral("docker"), QStringLiteral("podman")}) {
        const QString engine = BobPaths::findInPath(runtime);
        if (!engine.isEmpty())
            engines.append({runtime, engine});
    }
    if (engines.isEmpty()) {
        m_found.clear();
        emit finished();
        return;
    }
    m_pending = int(engines.size());
    for (const auto &found : engines) {
        const QString runtime = found.first;
        const QString engine = found.second;
        const QStringList args = {QStringLiteral("ps"), QStringLiteral("--quiet"), QStringLiteral("--filter"),
                                  QStringLiteral("label=%1").arg(launchLabel)};
        run(this, engine, args, [this, runtime, engine, round](std::optional<QByteArray> out) {
            if (round != m_round)
                return;
            const QStringList ids = out ? QString::fromUtf8(*out).split(QRegularExpression(QStringLiteral("\\s+")), Qt::SkipEmptyParts)
                                        : QStringList();
            if (ids.isEmpty())
                done(round);
            else
                inspect(runtime, engine, ids, round);
        });
    }
}

void RunningBobLister::inspect(const QString &runtime, const QString &engine, const QStringList &ids, quint64 round)
{
    run(this, engine, QStringList{QStringLiteral("inspect")} + ids, [this, runtime, round](std::optional<QByteArray> out) {
        if (round != m_round)
            return;
        if (out) {
            for (const QJsonValue &entry : QJsonDocument::fromJson(*out).array()) {
                if (auto bob = runningBob(entry.toObject(), runtime))
                    m_collecting.append(*bob);
            }
        }
        done(round);
    });
}

void RunningBobLister::done(quint64 round)
{
    if (round != m_round || --m_pending > 0)
        return;
    m_found = m_collecting;
    emit finished();
}
