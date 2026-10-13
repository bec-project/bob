#!/usr/bin/env bash
set -euo pipefail

# Only deliver a stable release tagged at this CI-tested commit.
: "${GITHUB_SHA:?Missing CI commit}"
: "${GITHUB_REPOSITORY:?Missing repository}"
: "${GITHUB_OUTPUT:?Missing workflow output file}"
task_ci=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
task_tag=$(git tag --points-at "$GITHUB_SHA" --list 'v[0-9]*' | \
  python3 "$task_ci/release_checks.py" tag)
if [[ -z "$task_tag" ]]; then
  echo 'release_available=false' >> "$GITHUB_OUTPUT"
  exit 0
fi
version=${task_tag#v}
: "${GITEA_REGISTRY_TOKEN:?Missing registry credentials}"
if [[ $(gh release view "$task_tag" --repo "$GITHUB_REPOSITORY" --json isDraft --jq .isDraft) != false ]]; then
  echo 'Release must be published before delivery' >&2
  exit 1
fi

task_files=$(mktemp -d)
trap 'rm -rf "$task_files"' EXIT
task_assets=()
for target in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  task_assets+=("bob-manager_${version}_${target}.tar.gz")
done
task_assets+=(SHA256SUMS)
task_patterns=()
for asset in "${task_assets[@]}"; do
  task_patterns+=(--pattern "$asset")
done
# Use the original assets, including on a fresh runner after a partial upload.
gh release download "$task_tag" --repo "$GITHUB_REPOSITORY" --dir "$task_files" "${task_patterns[@]}"
python3 "$task_ci/release_checks.py" checksums "$task_files" "$version"

# Feed credentials via stdin rather than exposing them in process arguments.
task_auth=$(printf '%s' "$GITEA_REGISTRY_TOKEN" | base64 | tr -d '\n')
for asset in "${task_assets[@]}"; do
  task_url="https://gitea.psi.ch/api/packages/bec/generic/bob/${version}/${asset}"
  task_status=$(printf 'header = "Authorization: Basic %s"\n' "$task_auth" | \
    curl --config - --silent --show-error --output "$task_files/response" \
      --write-out '%{http_code}' --upload-file "$task_files/$asset" "$task_url")
  case "$task_status" in
    201) ;;
    409)
      # A retry may encounter files already delivered by its previous attempt.
      printf 'header = "Authorization: Basic %s"\n' "$task_auth" | \
        curl --config - --fail --silent --show-error --output "$task_files/existing" "$task_url"
      if ! cmp -s "$task_files/$asset" "$task_files/existing"; then
        echo "Existing registry asset differs: $asset" >&2
        exit 1
      fi ;;
    *) echo "Registry upload failed for $asset (HTTP $task_status)" >&2; exit 1 ;;
  esac
done
printf 'release_available=true\nversion=%s\n' "$version" >> "$GITHUB_OUTPUT"
