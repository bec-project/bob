#!/usr/bin/env bash
set -euo pipefail

# Puts the Qt desktop app of a published release (the RHEL 9 RPM and the macOS DMG) into Gitea's
# generic package bob/VERSION, beside bob-manager, the RPM also into Gitea's RPM registry, and both
# onto the GitHub release. Gitea goes first, so a rerun that finds different files there stops
# before GitHub.
version=${1:?Usage: bash ci/publish_qt_desktop.sh VERSION DIRECTORY}
task_files=${2:?Usage: bash ci/publish_qt_desktop.sh VERSION DIRECTORY}
: "${GITHUB_REPOSITORY:?Missing repository}"
: "${GITEA_REGISTRY_TOKEN:?Missing registry credentials}"
task_tag="v$version"
if [[ $(gh release view "$task_tag" --repo "$GITHUB_REPOSITORY" --json isDraft --jq .isDraft) != false ]]; then
  echo 'Release must be published before delivery' >&2
  exit 1
fi

cd "$task_files"
shopt -s nullglob
task_rpms=("bob-gui-${version}-"*.rpm)
task_dmgs=("bob-gui_${version}_"*.dmg)
if [[ ${#task_rpms[@]} -ne 1 || ${#task_dmgs[@]} -ne 1 ]]; then
  echo 'Expected one RPM and one DMG' >&2
  exit 1
fi
task_rpm=${task_rpms[0]}
task_assets=("$task_rpm" "${task_dmgs[@]}")
task_sums="bob-gui_${version}_SHA256SUMS"
shasum -a 256 "${task_assets[@]}" > "$task_sums"
task_assets+=("$task_sums")

task_scratch=$(mktemp -d)
trap 'rm -rf "$task_scratch"' EXIT
# Feed credentials via stdin rather than exposing them in process arguments.
task_auth=$(printf '%s' "$GITEA_REGISTRY_TOKEN" | base64 | tr -d '\n')
task_curl() {
  printf 'header = "Authorization: Basic %s"\n' "$task_auth" | curl --config - "$@"
}
# Uploads FILE to URL; a file there already must be the same (a retry after a partial delivery).
task_deliver() { # FILE UPLOAD_URL DOWNLOAD_URL [METHOD]
  local status
  status=$(task_curl --silent --show-error --output "$task_scratch/response" --write-out '%{http_code}' \
    --request "${4:-PUT}" --upload-file "$1" "$2")
  case "$status" in
    201) ;;
    409)
      task_curl --fail --silent --show-error --output "$task_scratch/existing" "$3"
      if ! cmp -s "$1" "$task_scratch/existing"; then
        echo "Existing registry asset differs: $1" >&2
        exit 1
      fi ;;
    *) echo "Registry upload failed for $1 (HTTP $status)" >&2; exit 1 ;;
  esac
}
task_generic="https://gitea.psi.ch/api/packages/bec/generic/bob/${version}"
for asset in "${task_assets[@]}"; do
  task_deliver "$asset" "$task_generic/$asset" "$task_generic/$asset"
done
# bob-gui-1.2.3-1.el9.x86_64.rpm: the registry serves it as bob-gui, 1.2.3-1.el9, x86_64
task_arch=${task_rpm%.rpm}
task_arch=${task_arch##*.}
task_release=${task_rpm#bob-gui-}
task_release=${task_release%."$task_arch".rpm}
task_deliver "$task_rpm" "https://gitea.psi.ch/api/packages/bec/rpm/upload" \
  "https://gitea.psi.ch/api/packages/bec/rpm/package/bob-gui/$task_release/$task_arch"
gh release upload "$task_tag" --repo "$GITHUB_REPOSITORY" --clobber "${task_assets[@]}"
