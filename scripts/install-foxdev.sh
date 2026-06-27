#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/install-foxdev.sh [--check]

Build the current branch of foxharness-go into a `foxdev` binary and install
it alongside the released `fox`, so the latest in-development foxharness can be
run in any project directory for testing.

  fox      released version (e.g. /usr/local/bin/fox)
  foxdev   this branch's build  (e.g. /usr/local/bin/foxdev)

Switch to any feature branch and re-run this script to refresh `foxdev`.

Environment:
  PREFIX   Install directory. Default: /usr/local/bin
           Use PREFIX=~/go/bin to install without sudo (requires it on PATH).

Options:
  --check   Run `go test ./...` before building; abort the install if it fails.

Examples:
  scripts/install-foxdev.sh
  sudo scripts/install-foxdev.sh
  PREFIX=~/go/bin scripts/install-foxdev.sh
  scripts/install-foxdev.sh --check
EOF
}

check=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --check)
      check=true
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

# Locate the foxharness-go root by walking up to its go.mod.
root=""
dir="$(pwd)"
while [ "$dir" != "/" ]; do
  if [ -f "$dir/go.mod" ] && grep -q "module github.com/Zts0hg/foxharness" "$dir/go.mod" 2>/dev/null; then
    root="$dir"
    break
  fi
  dir="$(dirname "$dir")"
done
if [ -z "$root" ]; then
  echo "could not locate foxharness-go root (no go.mod for module github.com/Zts0hg/foxharness upward from $(pwd))" >&2
  exit 1
fi

# Expand a leading ~ so `PREFIX=~/go/bin` works even when quoted.
prefix="${PREFIX:-/usr/local/bin}"
prefix="${prefix/#\~/$HOME}"

branch="$(git -C "$root" rev-parse --abbrev-ref HEAD)"
commit="$(git -C "$root" rev-parse --short HEAD)"

echo "repo root:   $root"
echo "branch:      $branch"
echo "commit:      $commit"
echo "install dir: $prefix"

if [ "$check" = true ]; then
  echo "running go test ./..."
  (cd "$root" && go test ./...)
fi

if [ ! -d "$prefix" ]; then
  echo "install dir does not exist: $prefix" >&2
  echo "create it first, or choose another via PREFIX=..." >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "building foxdev..."
(cd "$root" && go build -trimpath -ldflags="-s -w" -o "$tmpdir/foxdev" ./cmd/fox)

if ! install -m 0755 "$tmpdir/foxdev" "$prefix/foxdev"; then
  echo "failed to install to $prefix" >&2
  echo "re-run with sudo for /usr/local/bin:" >&2
  echo "  sudo \"$0\" $*" >&2
  echo "or install without sudo into a writable directory:" >&2
  echo "  PREFIX=\"$HOME/go/bin\" \"$0\" $*" >&2
  exit 1
fi

case ":$PATH:" in
  *":$prefix:"*) ;;
  *)
    echo "note: $prefix is not on your PATH; add it before running foxdev" >&2
    ;;
esac

echo "installed foxdev -> $prefix/foxdev"
echo "run 'foxdev' in any project directory to test this branch"
