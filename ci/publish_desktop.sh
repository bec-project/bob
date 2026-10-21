#!/usr/bin/env bash
set -euo pipefail

# Puts the desktop app of a published release (the macOS DMG and the Linux Flatpak) into Gitea's
# generic package bob/VERSION, beside bob-manager, and onto its GitHub release. Gitea goes
# first, so a rerun that finds different files there stops before GitHub.
version=${1:?Usage: bash ci/publish_desktop.sh VERSION DIRECTORY}
task_files=${2:?Usage: bash ci/publish_desktop.sh VERSION DIRECTORY}
: "${GITHUB_REPOSITORY:?Missing repository}"
: "${GITEA_REGISTRY_TOKEN:?Missing registry credentials}"
task_tag="v$version"
if [[ $(gh release view "$task_tag" --repo "$GITHUB_REPOSITORY" --json isDraft --jq .isDraft) != false ]]; then
  echo 'Release must be published before delivery' >&2
  exit 1
fi

cd "$task_files"
shopt -s nullglob
task_dmgs=("bob-desktop_${version}_"*.dmg)
task_flatpaks=("bob-desktop_${version}_"*.flatpak)
if [[ ${#task_dmgs[@]} -ne 1 || ${#task_flatpaks[@]} -ne 1 ]]; then
  echo 'Expected one DMG and one Flatpak' >&2
  exit 1
fi
task_assets=("${task_dmgs[@]}" "${task_flatpaks[@]}")
task_sums="bob-desktop_${version}_SHA256SUMS"
shasum -a 256 "${task_assets[@]}" > "$task_sums"
task_assets+=("$task_sums")

task_scratch=$(mktemp -d)
trap 'rm -rf "$task_scratch"' EXIT
# Feed credentials via stdin rather than exposing them in process arguments.
task_auth=$(printf '%s' "$GITEA_REGISTRY_TOKEN" | base64 | tr -d '\n')
task_upload() { # FILE URL: prints the HTTP status
  printf 'header = "Authorization: Basic %s"\n' "$task_auth" | \
    curl --config - --silent --show-error --output "$task_scratch/response" \
      --write-out '%{http_code}' --upload-file "$1" "$2"
}
for asset in "${task_assets[@]}"; do
  task_url="https://gitea.psi.ch/api/packages/bec/generic/bob/${version}/${asset}"
  task_status=$(task_upload "$asset" "$task_url")
  case "$task_status" in
    201) ;;
    409)
      # A retry may encounter files already delivered by its previous attempt.
      printf 'header = "Authorization: Basic %s"\n' "$task_auth" | \
        curl --config - --fail --silent --show-error --output "$task_scratch/existing" "$task_url"
      if ! cmp -s "$asset" "$task_scratch/existing"; then
        echo "Existing registry asset differs: $asset" >&2
        exit 1
      fi ;;
    *) echo "Registry upload failed for $asset (HTTP $task_status)" >&2; exit 1 ;;
  esac
done
gh release upload "$task_tag" --repo "$GITHUB_REPOSITORY" --clobber "${task_assets[@]}"
