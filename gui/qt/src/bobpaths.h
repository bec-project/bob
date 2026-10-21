#pragma once

#include <QString>
#include <QStringList>

// Where bob's executable, configuration and container engines are found.
namespace BobPaths {

// PATH plus the folders bob and the engines are usually installed in: apps started from the
// Dock, Finder or a desktop menu get a minimal PATH.
QString enginePath();

// The first executable called `name` on the widened PATH, or an empty string.
QString findInPath(const QString &name);

bool isExecutable(const QString &path);

// The bob executable to use when the user has not chosen one: $BOB_BIN, bob-manager beside the
// app's own executable, build/bob-manager of the source tree (a build from a checkout only),
// then bob-manager or bob on the PATH. Empty when none is found.
QString defaultBinary();

// bob.toml at the root of the source tree, for a build from a checkout that has one.
QString defaultConfig();

// The folder holding the app's settings.json, shared with the Tauri app:
// ~/.config/ch.psi.bob on Linux, ~/Library/Application Support/ch.psi.bob on macOS, or
// $BOB_GUI_CONFIG_DIR.
QString configDir();

}  // namespace BobPaths
