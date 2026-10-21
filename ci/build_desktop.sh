#!/usr/bin/env bash
set -euo pipefail

# Builds the desktop app for release:
#   macos  an arm64 DMG with bob-manager beside the app (run on an arm64 Mac)
#   linux  the app's binary, bob-gui, for ci/build_flatpak.sh (run on Ubuntu 24.04 with
#          WebKitGTK 4.1; the Flatpak's GNOME runtime provides the same libraries)
# Expects `npm ci` to have run in gui/web.
platform=${1:?Usage: bash ci/build_desktop.sh macos|linux VERSION [DESTINATION]}
version=${2:?Usage: bash ci/build_desktop.sh macos|linux VERSION [DESTINATION]}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a stable semantic version' >&2
  exit 1
fi

task_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$task_root"
destination=${3:-dist}
mkdir -p "$destination"
destination=$(cd "$destination" && pwd)

task_target=${CARGO_TARGET_DIR:-$task_root/gui/desktop/target}
task_version="{\"version\": \"$version\"}"
case "$platform" in
  macos)
    [[ $(uname -sm) == 'Darwin arm64' ]] || { echo 'The DMG is built on an arm64 Mac' >&2; exit 1; }
    # Tauri copies binaries/bob-manager-<target triple> into the bundle as bob-manager.
    mkdir -p gui/desktop/binaries
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go -C launcher build -trimpath -ldflags='-s -w' \
      -o "$task_root/gui/desktop/binaries/bob-manager-aarch64-apple-darwin" ./cmd/container_manager
    rm -rf "$task_target/release/bundle"
    npm --prefix gui/web run tauri -- build --ci --bundles dmg \
      --config "$task_root/gui/desktop/tauri.release.conf.json" --config "$task_version"
    cp "$task_target"/release/bundle/dmg/*.dmg "$destination/bob-desktop_${version}_darwin_arm64.dmg" ;;
  linux)
    npm --prefix gui/web run tauri -- build --ci --no-bundle --config "$task_version"
    cp "$task_target/release/bob-gui" "$destination/bob-gui" ;;
  *) echo "Unknown platform $platform" >&2; exit 1 ;;
esac
ls -l "$destination"
