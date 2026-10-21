#!/usr/bin/env bash
set -euo pipefail

# Wraps the app's Linux binary (ci/build_desktop.sh linux) in a Flatpak bundle on the GNOME
# runtime, which brings WebKitGTK 4.1 to systems without it (RHEL 9). Needs flatpak-builder and
# a flathub remote, as in the flatpak-github-actions images.
#   bash ci/build_flatpak.sh VERSION BINARY [DESTINATION]
version=${1:?Usage: bash ci/build_flatpak.sh VERSION BINARY [DESTINATION]}
binary=${2:?Usage: bash ci/build_flatpak.sh VERSION BINARY [DESTINATION]}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a stable semantic version' >&2
  exit 1
fi
case $(uname -m) in
  x86_64) goarch=amd64 ;;
  aarch64) goarch=arm64 ;;
  *) echo "Unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

binary=$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")
task_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$task_root"
destination=${3:-dist}
mkdir -p "$destination"
destination=$(cd "$destination" && pwd)
task_build=$(mktemp -d)
trap 'rm -rf "$task_build"' EXIT

# The manifest finds its files beside itself and the icons in ../icons.
cp -r gui/desktop/flatpak gui/desktop/icons "$task_build/"
install -m 755 "$binary" "$task_build/flatpak/bob-gui"
flatpak-builder --disable-rofiles-fuse --force-clean --install-deps-from=flathub \
  --state-dir="$task_build/state" --repo="$task_build/repo" \
  "$task_build/app" "$task_build/flatpak/ch.psi.bob.yml"
flatpak build-bundle --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo \
  "$task_build/repo" "$destination/bob-desktop_${version}_linux_${goarch}.flatpak" ch.psi.bob
ls -l "$destination"
