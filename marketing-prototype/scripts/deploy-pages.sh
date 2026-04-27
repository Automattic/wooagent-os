#!/usr/bin/env bash
# Builds the prototype with the GHES Pages base path and force-pushes the
# static output to the `gh-pages` branch on origin. GitHub Actions is not
# enabled on the Automattic GHES instance, so this is a manual redeploy.
#
# Usage (from anywhere in the repo): bash marketing-prototype/scripts/deploy-pages.sh
#
# Pages serves the result at:
#   https://github.a8c.com/pages/Automattic/wooagent-os/

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PROJECT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
REPO_ROOT=$(cd "$PROJECT_DIR/.." && pwd)
BASE_PATH="/pages/Automattic/wooagent-os/"
REMOTE_URL=$(git -C "$REPO_ROOT" remote get-url origin)

echo "→ Building marketing-prototype with PUBLIC_BASE=$BASE_PATH"
cd "$PROJECT_DIR"
rm -rf dist
PUBLIC_BASE="$BASE_PATH" npm run build

echo "→ Preparing dist/ for gh-pages"
cd dist
cp index.html 404.html       # SPA deep-link fallback
touch .nojekyll              # Pages should serve files starting with `_`

SHA=$(git -C "$REPO_ROOT" rev-parse --short HEAD)

git init -q -b gh-pages
git config user.email "$(git -C "$REPO_ROOT" config user.email)"
git config user.name "$(git -C "$REPO_ROOT" config user.name)"
git add .
git commit -q -m "deploy: marketing-prototype @ $SHA"

echo "→ Force-pushing to $REMOTE_URL gh-pages"
git remote add origin "$REMOTE_URL"
git push -q --force origin gh-pages

cd "$PROJECT_DIR"
rm -rf dist

echo "✓ Deployed. Pages may take ~30s to refresh."
echo "  https://github.a8c.com/pages/Automattic/wooagent-os/"
