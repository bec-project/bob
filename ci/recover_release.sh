#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_SHA:?Missing CI commit}"
: "${GITHUB_REPOSITORY:?Missing repository}"
task_ci=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
task_tag=$(git tag --points-at "$GITHUB_SHA" --list 'v[0-9]*' | \
  python3 "$task_ci/release_checks.py" tag)
if [[ -z "$task_tag" ]]; then
  exit 0
fi
# A successful API query with no matching release means the tag was pushed first.
# API failures must never be mistaken for a missing release.
task_draft=$(gh api --paginate "repos/${GITHUB_REPOSITORY}/releases" \
  --jq ".[] | select(.tag_name == \"$task_tag\") | .draft")
case "$task_draft" in
  false) exit 0 ;; # Published assets are never rebuilt or replaced.
  true|'') ;;
  *) echo 'Ambiguous release state; refusing recovery' >&2; exit 1 ;;
esac
if [[ $(git rev-parse HEAD) != "$GITHUB_SHA" ]]; then
  echo 'Recovery must build the CI-tested commit' >&2
  exit 1
fi
version=${task_tag#v}
task_files=$(mktemp -d)
trap 'rm -rf "$task_files"' EXIT
# Partial draft assets were never published. Rebuild them as a consistent set.
bash "$task_ci/build_binaries.sh" "$version" "$task_files"
python3 "$task_ci/release_checks.py" checksums "$task_files" "$version"
if [[ -z "$task_draft" ]]; then
  gh release create "$task_tag" --repo "$GITHUB_REPOSITORY" --draft --verify-tag \
    --title "$task_tag" --generate-notes
fi
gh release upload "$task_tag" --repo "$GITHUB_REPOSITORY" --clobber \
  "$task_files/"*.tar.gz "$task_files/SHA256SUMS"
task_published=$(gh api --paginate "repos/${GITHUB_REPOSITORY}/releases" \
  --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name')
task_latest=$(printf '%s\n%s\n' "$task_published" "$task_tag" | \
  python3 "$task_ci/release_checks.py" latest "$version")
gh release edit "$task_tag" --repo "$GITHUB_REPOSITORY" --draft=false --latest="$task_latest"
