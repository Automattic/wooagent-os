#!/usr/bin/env bash
# Build build/wooagent-companion.zip — same packaging shape WordPress's
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
OUTPUT="$OUT_DIR/wooagent-companion.zip"

[ -d "$SRC_DIR" ] || { echo "missing $SRC_DIR" >&2; exit 1; }

mkdir -p "$OUT_DIR"
rm -f "$OUTPUT"
STAGE_DIR="$(mktemp -d "$OUT_DIR/companion-plugin-stage.XXXXXX")"

cleanup() {
	rm -rf -- "$STAGE_DIR"
}
trap cleanup EXIT

# Copy under a directory named for the plugin slug — WordPress unpacks the
# top-level directory directly into wp-content/plugins/, so the name has
# to match the plugin folder it'll create.
mkdir -p "$STAGE_DIR/wooagent-companion"
cp "$SRC_DIR/wooagent-companion.php" "$STAGE_DIR/wooagent-companion/"
cp "$SRC_DIR/readme.txt" "$STAGE_DIR/wooagent-companion/"
cp -R "$SRC_DIR/includes" "$STAGE_DIR/wooagent-companion/"
cp -R "$SRC_DIR/vendor" "$STAGE_DIR/wooagent-companion/"

# Strip Mac/editor cruft so the zip is lean and reproducible.
find "$STAGE_DIR" \( -name ".DS_Store" -o -name "*.swp" \) -delete

# Normalize timestamps and entry ordering, and omit host-specific extra
# attributes so identical source trees produce byte-identical archives.
find "$STAGE_DIR" -exec touch -t 198001010000 {} +
(
	cd "$STAGE_DIR"
	find wooagent-companion -print | LC_ALL=C sort | zip -q -X "$OUTPUT" -@
)

echo "Built $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
