#!/usr/bin/env bash
# Copy the built React UI from ui/dist/ into daemon/internal/uiassets/dist/
# so the next `go build` picks it up via //go:embed. Called from
# .goreleaser.yaml's before-hooks on releases; runnable standalone when
# iterating locally on the embedded UI flow.
#
# Workflow:
#   cd ui && npm install && npm run build
#   bash scripts/build-ui-into-daemon.sh
#   cd daemon && go build ./cmd/wooagent
#
# (For day-to-day UI iteration, prefer the Vite dev server on :5173 —
# the daemon's CORS already permits it.)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$REPO_ROOT/ui/dist"
TARGET="$REPO_ROOT/daemon/internal/uiassets/dist"

if [ ! -d "$SRC" ]; then
  printf "ui/dist not found — run \`cd ui && npm install && npm run build\` first.\n" >&2
  exit 1
fi

if [ ! -f "$SRC/index.html" ]; then
  printf "ui/dist/index.html missing — Vite build did not complete cleanly.\n" >&2
  exit 1
fi

mkdir -p "$(dirname "$TARGET")"
rm -rf "$TARGET"
cp -R "$SRC" "$TARGET"

# Strip macOS metadata so the embedded archive is reproducible.
find "$TARGET" -name ".DS_Store" -delete

printf "Copied %s of UI assets into %s\n" "$(du -sh "$TARGET" | cut -f1)" "$TARGET"
