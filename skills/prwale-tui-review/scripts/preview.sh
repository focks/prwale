#!/usr/bin/env bash
# Render prwale preview frames from fixture data (see preview_test.go).
# Usage: preview.sh [output-dir]   (default /tmp/prwale-preview)
set -euo pipefail
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
out="${1:-/tmp/prwale-preview}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
cd "$repo"
PRWALE_PREVIEW_DIR="$out" go test -count=1 -run '^TestRenderPreview$' . >/dev/null
ls "$out"/*.ans | wc -l | xargs echo "frames written to $out:"
echo "view one with: cat $out/120x40_all_overview.ans"
