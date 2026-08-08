#!/bin/sh
set -eu

missing=""
cache_dir="${GOCACHE:-${TMPDIR:-/tmp}/foxharness-go-build-cache}"
tmp_file="$(mktemp "${TMPDIR:-/tmp}/foxharness-package-tests.XXXXXX")"
trap 'rm -f "$tmp_file"' EXIT

mkdir -p "$cache_dir"
GOCACHE="$cache_dir" go list -f '{{.ImportPath}}|{{.Dir}}|{{len .TestGoFiles}}|{{len .XTestGoFiles}}' ./... >"$tmp_file"

while IFS='|' read -r import_path dir test_count external_test_count; do
	case "$dir" in
		*/vendor/*)
			continue
			;;
	esac
	if [ "$test_count" -eq 0 ] && [ "$external_test_count" -eq 0 ]; then
		missing="${missing}${import_path} (${dir})
"
	fi
done <"$tmp_file"

if [ -n "$missing" ]; then
	printf 'Go packages without package-level tests:\n%s' "$missing" >&2
	exit 1
fi
