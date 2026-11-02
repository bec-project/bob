#!/usr/bin/env bash
set -euo pipefail

# Builds the Qt desktop app for release on an arm64 Mac: an ad-hoc signed bob-gui.app with Qt inside,
# in DESTINATION/bob-gui_VERSION_darwin_arm64.dmg. Uses the Qt in QT_ROOT_DIR, or installs Qt's
# official binaries with aqtinstall. Needs Go on the PATH. The app also carries bob-manager, built
# here, in Contents/MacOS, and in Contents/libexec and Contents/lib what bob-manager runs: the
# pinned OpenCode, Node.js, ripgrep and sandbox-runtime (pinned by
# gui/qt/packaging/sandbox-runtime/package-lock.json), each checked against its checksum.
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
opencode_version=1.18.35
opencode_sha256=80b05124357a77cd57945bfde36082a028e829c198d222d5e146617f49a2c4b7
node_version=22.23.4
node_sha256=c9e1dbc748b33ed2d14f42445191ecaf14c56259c7a9a26d0042c49ec2b66428
ripgrep_version=14.1.1
ripgrep_sha256=24ad76777745fbff131c8fbc466742b011f925bfa4fffa2ded6def23b5b937be

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

# bob-manager beside the app's executable, where the app looks first, and what it runs bob with
# where it looks for them: Contents/libexec and Contents/lib
download() { # URL SHA256 FILE
  curl -fsSL -o "$3" "$1"
  echo "$2  $3" | shasum -a 256 -c --quiet
}
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C "$task_root/launcher" build -trimpath -ldflags='-s -w' \
  -o "$task_app/Contents/MacOS/bob-manager" ./cmd/bob-manager
mkdir -p "$task_app/Contents/libexec" "$task_app/Contents/lib/sandbox-runtime" "$task_top/downloads"
download "https://github.com/anomalyco/opencode/releases/download/v$opencode_version/opencode-darwin-arm64.zip" \
  "$opencode_sha256" "$task_top/downloads/opencode.zip"
unzip -q -o "$task_top/downloads/opencode.zip" opencode -d "$task_app/Contents/libexec"
download "https://nodejs.org/dist/v$node_version/node-v$node_version-darwin-arm64.tar.gz" \
  "$node_sha256" "$task_top/downloads/node.tar.gz"
tar -xzf "$task_top/downloads/node.tar.gz" -C "$task_top/downloads"
node_dir="$task_top/downloads/node-v$node_version-darwin-arm64"
cp "$node_dir/bin/node" "$task_app/Contents/libexec/node"
download "https://github.com/BurntSushi/ripgrep/releases/download/$ripgrep_version/ripgrep-$ripgrep_version-aarch64-apple-darwin.tar.gz" \
  "$ripgrep_sha256" "$task_top/downloads/ripgrep.tar.gz"
tar -xzf "$task_top/downloads/ripgrep.tar.gz" -C "$task_top/downloads"
ripgrep_dir="$task_top/downloads/ripgrep-$ripgrep_version-aarch64-apple-darwin"
cp "$ripgrep_dir/rg" "$task_app/Contents/libexec/rg"
cp "$task_root/gui/qt/packaging/sandbox-runtime/package.json" \
  "$task_root/gui/qt/packaging/sandbox-runtime/package-lock.json" "$task_app/Contents/lib/sandbox-runtime/"
PATH="$node_dir/bin:$PATH" npm ci --prefix "$task_app/Contents/lib/sandbox-runtime" \
  --omit=dev --no-audit --no-fund --ignore-scripts
rm "$task_app/Contents/lib/sandbox-runtime/package.json" "$task_app/Contents/lib/sandbox-runtime/package-lock.json"
# its Windows helper and Linux seccomp helpers are no use on a Mac
rm -rf "$task_app/Contents/lib/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime/vendor/srt-win" \
  "$task_app/Contents/lib/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime/vendor/seccomp"
licenses="$task_app/Contents/Resources/licenses"
mkdir -p "$licenses/opencode" "$licenses/node" "$licenses/ripgrep"
cp "$task_root/gui/qt/packaging/OPENCODE-LICENSE" "$licenses/opencode/LICENSE"
cp "$node_dir/LICENSE" "$licenses/node/LICENSE"
cp "$ripgrep_dir/LICENSE-MIT" "$licenses/ripgrep/LICENSE-MIT"
# arm64 Macs run only signed code: sign what came unsigned, before the app's own seal covers it
for program in "$task_app/Contents/MacOS/bob-manager" "$task_app/Contents/libexec/"*; do
  codesign --verify "$program" 2>/dev/null || codesign --force --sign - "$program"
done
# signed ad hoc, not with a developer certificate: macOS asks before the first start
codesign --force --deep --sign - "$task_app"
ln -s /Applications "$task_top/dmg/Applications"
hdiutil create -volname bob -srcfolder "$task_top/dmg" -ov -format UDZO \
  "$destination/bob-gui_${version}_darwin_arm64.dmg"
ls -l "$destination"
