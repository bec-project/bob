#!/usr/bin/env bash
set -euo pipefail

# Builds the Qt desktop app's x86_64 RPM for RHEL 9 into DESTINATION. Run it in an x86_64 EL9
# container (AlmaLinux or Rocky Linux 9) as root, with Go on the PATH: it installs the build
# dependencies, and Qt's official binaries with aqtinstall, which the RPM carries. (Qt's x86_64
# binaries are built on RHEL 9; its aarch64 ones need a newer glibc than RHEL 9 has.) The RPM
# also carries bob-manager, built here, the pinned OpenCode release, the sandbox-runtime it runs
# OpenCode under (pinned by gui/qt/packaging/sandbox-runtime/package-lock.json, installed with
# npm) and the pinned ripgrep release that sandbox-runtime scans with.
version=${1:?Usage: bash ci/build_qt_rpm.sh VERSION [DESTINATION]}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a semantic version' >&2
  exit 1
fi
task_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
destination=${2:-$task_root/dist}
mkdir -p "$destination"
destination=$(cd "$destination" && pwd)
qt_version=${QT_VERSION:-6.12.0}
# the OpenCode the sandbox runs, and its release archive's SHA-256
opencode_version=1.18.35
opencode_sha256=c8f888b451f5494a18f858fffb0e0b68f4e4baa9c241761c5f206884f0fa640d
ripgrep_version=14.1.1
ripgrep_sha256=4cf9f2741e6c465ffdb7c26f38056a59e2a2544b51f7cc128ef28337eeae4d8e
[[ $(uname -m) == x86_64 ]] || { echo 'The RPM is built on x86_64' >&2; exit 1; }
command -v go >/dev/null || { echo 'Go is needed to build bob-manager' >&2; exit 1; }

dnf -y -q install dnf-plugins-core rpm-build python3-pip
# npm installs sandbox-runtime; RHEL 9's default Node.js stream is too old for its lock file
dnf -y -q module enable nodejs:22
dnf -y -q install npm
dnf config-manager --set-enabled crb
dnf -y -q builddep "$task_root/gui/qt/packaging/bob-gui.spec"

task_top=$(mktemp -d)
trap 'rm -rf "$task_top"' EXIT
python3 -m venv "$task_top/aqt"
"$task_top/aqt/bin/pip" install -q aqtinstall==3.3.0
"$task_top/aqt/bin/aqt" install-qt linux desktop "$qt_version" linux_gcc_64 \
  --archives qtbase qtdeclarative qtsvg qtwayland icu --outputdir "$task_top/qt"
qt_prefix="$task_top/qt/$qt_version/gcc_64"

mkdir -p "$task_top/SOURCES"
CGO_ENABLED=0 go -C "$task_root/launcher" build -trimpath -ldflags='-s -w' \
  -o "$task_top/SOURCES/bob-manager" ./cmd/bob-manager
opencode_archive="$task_top/SOURCES/opencode-$opencode_version-linux-x64.tar.gz"
curl -fsSL -o "$opencode_archive" \
  "https://github.com/anomalyco/opencode/releases/download/v$opencode_version/opencode-linux-x64.tar.gz"
echo "$opencode_sha256  $opencode_archive" | sha256sum -c --quiet
cp "$task_root/gui/qt/packaging/OPENCODE-LICENSE" "$task_top/SOURCES/"
mkdir "$task_top/srt"
cp "$task_root/gui/qt/packaging/sandbox-runtime/package.json" \
  "$task_root/gui/qt/packaging/sandbox-runtime/package-lock.json" "$task_top/srt/"
npm ci --prefix "$task_top/srt" --omit=dev --no-audit --no-fund --ignore-scripts
# Bob refuses SRT's permissive missing-helper fallback; do not publish an incomplete runtime.
test -x "$task_top/srt/node_modules/@anthropic-ai/sandbox-runtime/vendor/seccomp/x64/apply-seccomp"
tar -C "$task_top/srt" -czf "$task_top/SOURCES/sandbox-runtime.tar.gz" node_modules
ripgrep_archive="$task_top/SOURCES/ripgrep-$ripgrep_version-x86_64.tar.gz"
curl -fsSL -o "$ripgrep_archive" \
  "https://github.com/BurntSushi/ripgrep/releases/download/$ripgrep_version/ripgrep-$ripgrep_version-x86_64-unknown-linux-musl.tar.gz"
echo "$ripgrep_sha256  $ripgrep_archive" | sha256sum -c --quiet
# the source tarball holds gui/qt as bob-gui-VERSION/
tar -C "$task_root/gui" --transform "s|^qt|bob-gui-$version|" --exclude-vcs --exclude='*.user' \
  --exclude='build*' -czf "$task_top/SOURCES/bob-gui-$version.tar.gz" qt
rpmbuild -bb --define "_topdir $task_top" --define "version_override $version" \
  --define "qt_prefix $qt_prefix" --define "opencode_version $opencode_version" \
  --define "ripgrep_version $ripgrep_version" \
  "$task_root/gui/qt/packaging/bob-gui.spec"
find "$task_top/RPMS" -name 'bob-gui-*.rpm' -exec cp {} "$destination/" \;
ls -l "$destination"
