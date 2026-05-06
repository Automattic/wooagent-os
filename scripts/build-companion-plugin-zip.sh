#!/usr/bin/env bash
# Build dist/wooagent-companion.zip — same packaging shape WordPress's
# plugin uploader expects: a single top-level wooagent-companion/ directory
# containing the plugin's PHP files. Called from .goreleaser.yaml's
# before-hooks so cutting a tag publishes the plugin alongside the daemon
# binaries; also runnable standalone for local testing.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="$REPO_ROOT/companion-plugin"
# Write to build/ rather than dist/ — dist/ is owned by goreleaser, which
# refuses to start if it's non-empty. build/ is gitignored separately so
# the plugin zip and the daemon binaries don't fight over the same dir.
OUT_DIR="$REPO_ROOT/build"
STAGE_DIR="$OUT_DIR/companion-plugin-stage"
OUTPUT="$OUT_DIR/wooagent-companion.zip"

[ -d "$SRC_DIR" ] || { echo "missing $SRC_DIR" >&2; exit 1; }

mkdir -p "$OUT_DIR"
rm -rf "$STAGE_DIR" "$OUTPUT"

# Copy under a directory named for the plugin slug — WordPress unpacks the
# top-level directory directly into wp-content/plugins/, so the name has
# to match the plugin folder it'll create.
mkdir -p "$STAGE_DIR/wooagent-companion"
cp -R "$SRC_DIR/." "$STAGE_DIR/wooagent-companion/"

# Strip Mac/editor cruft so the zip is lean and reproducible.
find "$STAGE_DIR" \( -name ".DS_Store" -o -name "*.swp" \) -delete

( cd "$STAGE_DIR" && zip -rq "$OUTPUT" wooagent-companion )

rm -rf "$STAGE_DIR"

echo "Built $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
