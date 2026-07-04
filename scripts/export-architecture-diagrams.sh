#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/export-architecture-diagrams.sh

Export architecture draw.io source pages into PNG images referenced by the
Markdown architecture documents.

Environment:
  DRAWIO_BIN  Path to the draw.io Desktop executable. Optional.

The script also searches these common locations:
  - drawio on PATH
  - /Applications/draw.io.app/Contents/MacOS/draw.io
  - /Applications/drawio.app/Contents/MacOS/drawio

Examples:
  scripts/export-architecture-diagrams.sh
  DRAWIO_BIN="/Applications/draw.io.app/Contents/MacOS/draw.io" scripts/export-architecture-diagrams.sh
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ "$#" -gt 0 ]; then
  echo "unknown argument: $1" >&2
  usage >&2
  exit 2
fi

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
  echo "could not locate foxharness-go root (no matching go.mod upward from $(pwd))" >&2
  exit 1
fi

find_drawio() {
  if [ -n "${DRAWIO_BIN:-}" ]; then
    printf '%s\n' "$DRAWIO_BIN"
    return
  fi
  if command -v drawio >/dev/null 2>&1; then
    command -v drawio
    return
  fi
  if [ -x "/Applications/draw.io.app/Contents/MacOS/draw.io" ]; then
    printf '%s\n' "/Applications/draw.io.app/Contents/MacOS/draw.io"
    return
  fi
  if [ -x "/Applications/drawio.app/Contents/MacOS/drawio" ]; then
    printf '%s\n' "/Applications/drawio.app/Contents/MacOS/drawio"
    return
  fi
}

drawio_bin="$(find_drawio || true)"
if [ -z "$drawio_bin" ] || [ ! -x "$drawio_bin" ]; then
  cat >&2 <<'EOF'
draw.io Desktop executable was not found.

Install draw.io Desktop or set DRAWIO_BIN to its executable path, then rerun:

  DRAWIO_BIN="/Applications/draw.io.app/Contents/MacOS/draw.io" scripts/export-architecture-diagrams.sh

EOF
  exit 127
fi

images_dir="$root/docs/architecture/images"
mkdir -p "$images_dir"

export_page() {
  local source="$1"
  local page_index="$2"
  local output="$3"

  echo "exporting ${source} page ${page_index} -> ${output}"
  "$drawio_bin" \
    --export \
    --format png \
    --page-index "$page_index" \
    --border 20 \
    --output "$output" \
    "$source"
}

export_page \
  "$root/docs/architecture/drawio/current-architecture.zh-CN.drawio" \
  0 \
  "$images_dir/current-architecture-a1-system-layers.zh-CN.png"

echo "architecture diagram export complete"
