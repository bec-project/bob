#!/usr/bin/env bash
set -euo pipefail

# Builds the Qt desktop app for release on an arm64 Mac: an ad-hoc signed bob-gui.app with Qt inside,
# in DESTINATION/bob-gui_VERSION_darwin_arm64.dmg. Uses the Qt in QT_ROOT_DIR, or installs Qt's
# official binaries with aqtinstall. bob-manager is installed separately.
version=${1:?Usage: bash ci/build_qt_macos.sh VERSION [DESTINATION]}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a semantic version' >&2
  exit 1
fi
[[ $(uname -sm) == 'Darwin arm64' ]] || { echo 'The DMG is built on an arm64 Mac' >&2; exit 1; }
task_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
destination=${2:-$task_root/dist}
mkdir -p "$destination"
destination=$(cd "$destination" && pwd)
qt_version=${QT_VERSION:-6.12.0}

task_top=$(mktemp -d)
trap 'rm -rf "$task_top"' EXIT
if [[ -z "${QT_ROOT_DIR:-}" ]]; then
  python3 -m venv "$task_top/aqt"
  "$task_top/aqt/bin/pip" install -q aqtinstall==3.3.0
  "$task_top/aqt/bin/aqt" install-qt mac desktop "$qt_version" clang_64 \
    --archives qtbase qtdeclarative qtsvg --outputdir "$task_top/qt"
  QT_ROOT_DIR="$task_top/qt/$qt_version/macos"
fi

task_build="$task_top/build"
cmake -S "$task_root/gui/qt" -B "$task_build" -G Ninja -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_PREFIX_PATH="$QT_ROOT_DIR" -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DBOB_VERSION="$version" -DBOB_SOURCE_DEFAULTS=OFF
cmake --build "$task_build"
(cd "$task_build" && QT_QPA_PLATFORM=offscreen ctest --output-on-failure)

task_app="$task_top/dmg/bob-gui.app"
mkdir -p "$task_top/dmg"
cp -R "$task_build/bob-gui.app" "$task_app"
"$QT_ROOT_DIR/bin/macdeployqt" "$task_app" -qmldir="$task_root/gui/qt/imports" -always-overwrite
# QML's LocalStorage brings Qt's SQL drivers along; the app uses none, and they want client
# libraries a Mac does not have
rm -rf "$task_app/Contents/PlugIns/sqldrivers"
# signed ad hoc, not with a developer certificate: macOS asks before the first start
codesign --force --deep --sign - "$task_app"
ln -s /Applications "$task_top/dmg/Applications"
hdiutil create -volname bob -srcfolder "$task_top/dmg" -ov -format UDZO \
  "$destination/bob-gui_${version}_darwin_arm64.dmg"
ls -l "$destination"
