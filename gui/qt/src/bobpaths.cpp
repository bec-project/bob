#include "bobpaths.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QStandardPaths>

namespace BobPaths {

QString enginePath()
{
    QStringList dirs;
    for (const QString &dir : qEnvironmentVariable("PATH").split(QLatin1Char(':'))) {
        if (!dir.isEmpty() && !dirs.contains(dir))
            dirs << dir;
    }
    const QString home = QDir::homePath();
    const QStringList extras = {
        QStringLiteral("/usr/local/bin"), QStringLiteral("/opt/homebrew/bin"),
        QStringLiteral("/opt/podman/bin"), QStringLiteral("/usr/bin"), QStringLiteral("/bin"),
        home + QStringLiteral("/.docker/bin"), home + QStringLiteral("/go/bin"),
        home + QStringLiteral("/.local/bin"),
    };
    for (const QString &dir : extras) {
        if (!dirs.contains(dir))
            dirs << dir;
    }
    return dirs.join(QLatin1Char(':'));
}

bool isExecutable(const QString &path)
{
    const QFileInfo info(path);
    return !path.isEmpty() && info.isFile() && info.isExecutable();
}

QString findInPath(const QString &name)
{
    for (const QString &dir : enginePath().split(QLatin1Char(':'))) {
        const QString candidate = QDir(dir).filePath(name);
        if (isExecutable(candidate))
            return candidate;
    }
    return {};
}

QString defaultBinary()
{
    const QString fromEnv = qEnvironmentVariable("BOB_BIN");
    if (isExecutable(fromEnv))
        return fromEnv;
    if (QCoreApplication::instance()) {
        const QString beside = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("bob-manager"));
        if (isExecutable(beside))
            return beside;
    }
#ifdef BOB_SOURCE_ROOT
    const QString built = QStringLiteral(BOB_SOURCE_ROOT "/build/bob-manager");
    if (isExecutable(built))
        return QFileInfo(built).canonicalFilePath();
#endif
    for (const QString &name : {QStringLiteral("bob-manager"), QStringLiteral("bob")}) {
        const QString found = findInPath(name);
        if (!found.isEmpty())
            return found;
    }
    return {};
}

QString defaultConfig()
{
#ifdef BOB_SOURCE_ROOT
    const QFileInfo config(QStringLiteral(BOB_SOURCE_ROOT "/bob.toml"));
    if (config.isFile())
        return config.canonicalFilePath();
#endif
    return {};
}

QString configDir()
{
    const QString fromEnv = qEnvironmentVariable("BOB_GUI_CONFIG_DIR");
    if (!fromEnv.isEmpty())
        return fromEnv;
#ifdef Q_OS_MACOS
    const auto base = QStandardPaths::GenericDataLocation;
#else
    const auto base = QStandardPaths::GenericConfigLocation;
#endif
    return QDir(QStandardPaths::writableLocation(base)).filePath(QStringLiteral("ch.psi.bob"));
}

}  // namespace BobPaths
