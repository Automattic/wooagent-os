#!/usr/bin/env bash
# Run the daemon from whichever worktree you're standing in.
#
#   bash scripts/dev-daemon.sh          # stop whatever holds the port, then run
#   bash scripts/dev-daemon.sh --keep   # refuse to stop it; just report and exit
#
# Why this exists: the repo uses git worktrees (`git worktree list` shows the
# root plus one directory per Conductor workspace), and it's easy to assume a
# workspace is a separate clone that needs its changes pulled somewhere before
# the daemon can see them. It isn't, twice over:
#
#   1. All worktrees share one .git, so a commit in any workspace is visible
#      from all of them immediately — no fetch, no pull.
#   2. Daemon state lives in ~/.wooagent (config.yaml, wooagent.db, logs,
#      ui-session.token), not in the checkout. config.DefaultPaths() hardcodes
#      the home dir and PathsAt() is test-only, so there's no env override and
#      no per-worktree state. Every worktree talks to the same database and the
#      same paired store.
#
# Which means the daemon can just run from here. The only thing in the way is
# the single bind port, so that's all this script handles.
#
# Don't `git checkout <branch>` in the root worktree to pick up a workspace's
# work — git refuses to check out a branch another worktree already holds, and
# you don't need to anyway. Run from the workspace.
#
# The UI is separate: `cd ui && npm run dev` on :5173, which the daemon's CORS
# already permits. Nothing to coordinate between the two.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$HOME/.wooagent/config.yaml"

KEEP=0
if [[ "${1:-}" == "--keep" ]]; then
  KEEP=1
elif [[ -n "${1:-}" ]]; then
  echo "unknown argument: $1 (expected --keep or nothing)" >&2
  exit 2
fi

# Read bind_addr from the same config the daemon reads, so this script
# follows a changed port instead of hardcoding 7777 and lying about it.
BIND_ADDR="localhost:7777"
if [[ -f "$CONFIG" ]]; then
  from_config="$(sed -n 's/^bind_addr:[[:space:]]*//p' "$CONFIG" | head -1)"
  [[ -n "$from_config" ]] && BIND_ADDR="$from_config"
fi
PORT="${BIND_ADDR##*:}"

echo "worktree:  $REPO_ROOT"
echo "branch:    $(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
echo "bind:      $BIND_ADDR"
echo "state:     $HOME/.wooagent  (shared across all worktrees)"
echo

# Only ever consider a process that is actually a wooagent daemon. Killing
# whatever happens to hold the port would be a genuinely bad surprise if
# something unrelated grabbed it.
holder_pid="$(lsof -ti "tcp:$PORT" -sTCP:LISTEN 2>/dev/null | head -1 || true)"
if [[ -n "$holder_pid" ]]; then
  holder_cmd="$(ps -o comm= -p "$holder_pid" 2>/dev/null || true)"
  holder_cwd="$(lsof -a -p "$holder_pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1 || true)"

  if [[ "$holder_cmd" != *wooagent* ]]; then
    echo "Port $PORT is held by pid $holder_pid ($holder_cmd), which isn't a" >&2
    echo "wooagent daemon. Not touching it — sort that out first." >&2
    exit 1
  fi

  echo "Existing daemon on :$PORT — pid $holder_pid, running from:"
  echo "  ${holder_cwd:-<unknown>}"

  if (( KEEP )); then
    echo
    echo "--keep given, so leaving it alone. Nothing started."
    exit 0
  fi

  echo "Stopping it (SIGTERM) so this worktree can bind..."
  kill "$holder_pid"

  # Wait for the port to actually free up. Binding immediately after SIGTERM
  # races the old process's shutdown, and `wooagent run` probes the bind
  # before doing setup work, so losing that race just fails the start.
  for _ in $(seq 1 40); do
    lsof -ti "tcp:$PORT" -sTCP:LISTEN >/dev/null 2>&1 || break
    sleep 0.25
  done
  if lsof -ti "tcp:$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Port $PORT still held after 10s. Check: lsof -i :$PORT" >&2
    exit 1
  fi
  echo "Port free."
  echo
fi

echo "Starting daemon from this worktree — Ctrl-C to stop."
echo
cd "$REPO_ROOT/daemon"
exec go run ./cmd/wooagent run
