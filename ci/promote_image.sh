#!/usr/bin/env bash
set -euo pipefail

version=${1:?Usage: bash ci/promote_image.sh VERSION}
: "${GITHUB_REPOSITORY:?Missing repository}"
task_ci=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# Called after the versioned image is published, under the publication job's lock.
# Check all published stable releases, not merely GitHub's configurable latest alias.
task_latest=$(gh api --paginate "repos/${GITHUB_REPOSITORY}/releases" \
  --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name' | \
  python3 "$task_ci/release_checks.py" latest "$version")
if [[ "$task_latest" == true ]]; then
  docker buildx imagetools create --tag gitea.psi.ch/bec/bob:latest "gitea.psi.ch/bec/bob:$version"
else
  echo "Skipping latest promotion for older release $version"
fi
