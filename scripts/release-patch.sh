#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/release-patch.sh [--dry-run]

Bump the latest vMAJOR.MINOR.PATCH tag by one patch version and push the new
tag to trigger the GitHub release workflow.

Environment:
  REMOTE          Git remote to fetch and push. Default: origin
  RELEASE_BRANCH Remote branch to tag. Default: main

Examples:
  scripts/release-patch.sh --dry-run
  scripts/release-patch.sh
EOF
}

dry_run=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      dry_run=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

remote="${REMOTE:-origin}"
release_branch="${RELEASE_BRANCH:-main}"

git fetch "$remote" "$release_branch" --tags

remote_ref="$remote/$release_branch"
remote_sha="$(git rev-parse "$remote_ref")"
latest_tag="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1)"

if [ -z "$latest_tag" ]; then
  echo "no semver tag found matching vMAJOR.MINOR.PATCH" >&2
  exit 1
fi

version="${latest_tag#v}"
IFS=. read -r major minor patch <<EOF
$version
EOF

case "$major.$minor.$patch" in
  *[!0-9.]*|.*|*..*|*.)
    echo "latest tag is not a simple semver patch tag: $latest_tag" >&2
    exit 1
    ;;
esac

next_tag="v${major}.${minor}.$((patch + 1))"

if git rev-parse -q --verify "refs/tags/$next_tag" >/dev/null; then
  echo "tag already exists locally: $next_tag" >&2
  exit 1
fi

if git ls-remote --exit-code --tags "$remote" "refs/tags/$next_tag" >/dev/null 2>&1; then
  echo "tag already exists on $remote: $next_tag" >&2
  exit 1
fi

echo "remote branch: $remote_ref"
echo "target commit: $remote_sha"
echo "latest tag:    $latest_tag"
echo "next tag:      $next_tag"

if [ "$dry_run" = true ]; then
  echo "dry run only; no tag created or pushed"
  exit 0
fi

git tag -a "$next_tag" "$remote_sha" -m "$next_tag"
git push "$remote" "$next_tag"

echo "pushed $next_tag; GitHub Actions release workflow should start shortly"
