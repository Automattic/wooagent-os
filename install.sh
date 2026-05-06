#!/usr/bin/env bash
# WooAgent OS — one-line installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/elizaan36/wooagent-os/trunk/install.sh | bash
#
# Pin a version:
#   curl -fsSL .../install.sh | WOOAGENT_VERSION=v0.1.0 bash
#
# Install to a different location:
#   curl -fsSL .../install.sh | WOOAGENT_INSTALL_DIR=/usr/local/bin bash
#
# Use a different repo (private fork, internal mirror, etc.):
#   curl -fsSL .../install.sh | WOOAGENT_REPO=Automattic/wooagent-os bash
#
# What it does:
#   1. Detect OS + arch
#   2. Resolve the latest GitHub release (or honor WOOAGENT_VERSION)
#   3. Download the matching tarball
#   4. Verify SHA256 against the release's SHA256SUMS file
#   5. Install the binary to ~/.wooagent/bin/wooagent
#   6. Print a PATH hint if ~/.wooagent/bin isn't on PATH

set -euo pipefail

REPO="${WOOAGENT_REPO:-elizaan36/wooagent-os}"
VERSION="${WOOAGENT_VERSION:-latest}"
INSTALL_DIR="${WOOAGENT_INSTALL_DIR:-$HOME/.wooagent/bin}"

err() { printf >&2 "\033[31merror:\033[0m %s\n" "$*"; exit 1; }
log() { printf "  %s\n" "$*"; }

printf "\n\033[1mWooAgent OS installer\033[0m\n\n"

# ---------- detect platform ----------
case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  *)      err "unsupported OS: $(uname -s). Windows users: download the .zip from the GitHub Releases page or use WSL." ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  arm64 | aarch64) ARCH=arm64 ;;
  *) err "unsupported architecture: $(uname -m)" ;;
esac

log "Platform: $OS/$ARCH"

# ---------- pick a checksum tool ----------
if command -v shasum >/dev/null 2>&1; then
  CHECKSUM_CMD="shasum -a 256"
elif command -v sha256sum >/dev/null 2>&1; then
  CHECKSUM_CMD="sha256sum"
else
  err "neither shasum nor sha256sum found — cannot verify download"
fi

# ---------- resolve version ----------
if [ "$VERSION" = "latest" ]; then
  log "Resolving latest release of $REPO..."
  LATEST_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest") \
    || err "could not reach GitHub releases API. Override with WOOAGENT_VERSION if rate-limited."
  VERSION=$(printf '%s' "$LATEST_JSON" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$VERSION" ] || err "could not determine latest version from GitHub API response"
fi
log "Version:  $VERSION"

VERSION_NUM="${VERSION#v}"
ARCHIVE="wooagent_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
SUMS_URL="https://github.com/$REPO/releases/download/$VERSION/SHA256SUMS"

# ---------- download + verify ----------
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

log "Downloading $ARCHIVE"
curl -fsSL "$URL" -o "$TMP/$ARCHIVE" \
  || err "download failed: $URL  (the release may not include a build for $OS/$ARCH)"

log "Verifying checksum"
curl -fsSL "$SUMS_URL" -o "$TMP/SHA256SUMS" \
  || err "checksum file unavailable at $SUMS_URL"

EXPECTED=$(grep " $ARCHIVE\$" "$TMP/SHA256SUMS" | cut -d' ' -f1)
[ -n "$EXPECTED" ] || err "no checksum entry for $ARCHIVE in SHA256SUMS"
ACTUAL=$(cd "$TMP" && $CHECKSUM_CMD "$ARCHIVE" | cut -d' ' -f1)
[ "$EXPECTED" = "$ACTUAL" ] || err "checksum mismatch: expected $EXPECTED, got $ACTUAL"

# ---------- install ----------
log "Extracting"
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"

mkdir -p "$INSTALL_DIR"
mv "$TMP/wooagent" "$INSTALL_DIR/wooagent"
chmod +x "$INSTALL_DIR/wooagent"

# Strip the macOS quarantine flag so the binary doesn't trip Gatekeeper
# when the operator runs it. Best-effort — fails silently on Linux.
if [ "$OS" = "darwin" ]; then
  xattr -d com.apple.quarantine "$INSTALL_DIR/wooagent" 2>/dev/null || true
fi

printf "\n\033[32m✓\033[0m Installed wooagent %s to %s/wooagent\n" "$VERSION" "$INSTALL_DIR"

# ---------- PATH hint ----------
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    printf "\nRun \033[1mwooagent run\033[0m to start the daemon.\n\n"
    ;;
  *)
    SHELL_NAME=$(basename "${SHELL:-}")
    case "$SHELL_NAME" in
      zsh)  RC="~/.zshrc" ;;
      bash) RC="~/.bashrc" ;;
      *)    RC="your shell profile" ;;
    esac
    cat <<EOF

To finish installing, add this to $RC:

  export PATH="$INSTALL_DIR:\$PATH"

Then in a new shell:

  wooagent run

EOF
    ;;
esac
