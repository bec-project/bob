// bob's desktop app: a window that starts or joins the user's bob and chats with the OpenCode
// server in its container.

#include "bob.h"
#include "bobpaths.h"
#include "chat.h"
#include "prefs.h"

#include <QDir>
#include <QGuiApplication>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQmlExtensionPlugin>
#include <QQuickStyle>

Q_IMPORT_QML_PLUGIN(Bob_BackendPlugin)

int main(int argc, char *argv[])
{
    QGuiApplication app(argc, argv);
    QGuiApplication::setApplicationName(QStringLiteral("bob"));
    QGuiApplication::setApplicationDisplayName(QStringLiteral("bob"));
    QGuiApplication::setOrganizationName(QStringLiteral("PSI"));
    QGuiApplication::setOrganizationDomain(QStringLiteral("psi.ch"));
    QGuiApplication::setApplicationVersion(QStringLiteral(BOB_VERSION));
    QGuiApplication::setDesktopFileName(QStringLiteral("ch.psi.bob"));
    QGuiApplication::setWindowIcon(QIcon(QStringLiteral(":/qt/qml/Bob/images/bob.png")));
    if (qEnvironmentVariableIsEmpty("QT_QUICK_CONTROLS_STYLE"))
        QQuickStyle::setStyle(QStringLiteral("Basic"));

    // The lease on bob is an open connection: it ends with the app, however it exits. Joining
    // starts before the window loads; the window shows the status it finds.
    Bob bob(QDir(BobPaths::configDir()).filePath(QStringLiteral("settings.json")));
    Bob::setInstance(&bob);
    bob.joinOnLaunch();
    Prefs prefs(nullptr);
    Prefs::setInstance(&prefs);
    Chat chat(&bob, &prefs);
    Chat::setInstance(&chat);

    QQmlApplicationEngine engine;
    QObject::connect(&engine, &QQmlApplicationEngine::objectCreationFailed, &app, [] { QCoreApplication::exit(1); },
                     Qt::QueuedConnection);
    engine.loadFromModule("Bob", "Main");
    return QGuiApplication::exec();
}
