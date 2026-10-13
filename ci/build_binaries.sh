#!/usr/bin/env bash
set -euo pipefail

version=${1:?Usage: bash ci/build_binaries.sh VERSION [DESTINATION]}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a stable semantic version' >&2
  exit 1
fi

task_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$task_root"
destination=${2:-dist}
mkdir -p "$destination"
task_build=$(mktemp -d)
trap 'rm -rf "$task_build"' EXIT

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  platform=${target%/*}
  architecture=${target#*/}
  CGO_ENABLED=0 GOOS="$platform" GOARCH="$architecture" \
    go build -trimpath -ldflags='-s -w' -o "$task_build/bob-manager" ./cmd/container_manager
  tar -C "$task_build" -czf "$destination/bob-manager_${version}_${platform}_${architecture}.tar.gz" bob-manager
done

(
  cd "$destination"
  shasum -a 256 "bob-manager_${version}_"*.tar.gz > SHA256SUMS
)
