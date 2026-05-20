#!/usr/bin/env bash
# Refresh daemon/internal/manifest/default.json against the connected
# staging store. Surgically merges so placeholder entries (pre-signed
# canonical names per DSGWOO-1279) survive a refresh that runs against a
# store that doesn't yet register them. Without the merge, a naive
# clobber would drop the placeholders and a paired store running WC 10.9
# core would hit PEP denials on first invocation.
#
# Required env:
#   WOOAGENT_STORE             Store URL (e.g. https://shop.example.com)
#   WOOAGENT_MCP_USER          WordPress username or email
#   WOOAGENT_MCP_APP_PASSWORD  WordPress Application Password (spaces ok)
#
# Optional env:
#   WOOAGENT_FILTER            Comma-separated namespace prefixes
#
# Cadence: monthly, or whenever a plugin update on the connected store
# changes the ability surface. See CLAUDE.md "Manifest refresh".

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="$REPO_ROOT/daemon/internal/manifest/default.json"
: "${WOOAGENT_STORE:?WOOAGENT_STORE is required (e.g. https://shop.example.com)}"
: "${WOOAGENT_MCP_USER:?WOOAGENT_MCP_USER is required}"
: "${WOOAGENT_MCP_APP_PASSWORD:?WOOAGENT_MCP_APP_PASSWORD is required}"

STORE="$WOOAGENT_STORE"
FILTER="${WOOAGENT_FILTER:-woocommerce/*,wooagent-*,core/*,jetpack-forms/*}"
PLACEHOLDER='sha256:0000000000000000000000000000000000000000000000000000000000000000'

command -v jq  >/dev/null || { echo "jq not found"  >&2; exit 1; }
command -v go  >/dev/null || { echo "go not found"  >&2; exit 1; }

TMP_FRESH="$(mktemp -t manifest-fresh.XXXXXX.json)"
trap 'rm -f "$TMP_FRESH"' EXIT

echo "→ fetching live abilities from $STORE"
( cd "$REPO_ROOT/daemon" && go run ./cmd/manifest-compute \
    -store "$STORE" \
    -filter "$FILTER" \
    -pretty=true > "$TMP_FRESH" )

# Pull placeholder entries from the existing manifest. These are pre-signed
# canonical names that haven't been observed on a live store yet — drop
# them and an operator-facing 401 wall appears the moment WC 10.9 reaches
# a paired store.
PLACEHOLDERS_JSON="$(jq --arg ph "$PLACEHOLDER" \
    '[.entries[] | select(.schema_hash == $ph)]' "$MANIFEST")"
PLACEHOLDER_NAMES="$(echo "$PLACEHOLDERS_JSON" | jq -r '.[].ability')"

if [ -n "$PLACEHOLDER_NAMES" ]; then
    echo "→ preserving placeholder entries (DSGWOO-1279 pre-signs):"
    echo "$PLACEHOLDER_NAMES" | sed 's/^/    /'
fi

# Merge: fresh entries (real schemas from the live store), then placeholder
# entries that aren't in fresh (they haven't been observed yet — keep them).
MERGED="$(jq -s --arg ph "$PLACEHOLDER" '
    .[0] as $fresh | .[1] as $placeholders
    | ($fresh.entries | map(.ability)) as $fresh_names
    | $fresh
    | .entries += ($placeholders | map(select(.ability as $a | $fresh_names | index($a) | not)))
' "$TMP_FRESH" <(echo "$PLACEHOLDERS_JSON"))"

echo "$MERGED" > "$MANIFEST"

FINAL_COUNT="$(jq '.entries | length' "$MANIFEST")"
echo "→ wrote $MANIFEST ($FINAL_COUNT entries)"

echo "→ running manifest tests"
( cd "$REPO_ROOT/daemon" && go test ./internal/manifest/... )

echo "→ done. Review the diff (git diff daemon/internal/manifest/default.json) before committing."
