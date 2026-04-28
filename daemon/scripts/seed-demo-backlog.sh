#!/usr/bin/env bash
# Seeds three marketing-agent backlog items for the demo board.
#
# Usage:
#   WOOAGENT_DAEMON_TOKEN=$(wooagent auth token create | tail -1) \
#   bash daemon/scripts/seed-demo-backlog.sh
#
# Optional: WOOAGENT_DAEMON_URL (default: http://localhost:7777)

set -euo pipefail

URL="${WOOAGENT_DAEMON_URL:-http://localhost:7777}"
TOKEN="${WOOAGENT_DAEMON_TOKEN:?set WOOAGENT_DAEMON_TOKEN — try: wooagent auth token create}"

post() {
  local title="$1" description="$2"
  curl -fsS -X POST "$URL/v1/issues" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg title "$title" \
      --arg description "$description" \
      '{title: $title, description: $description, persona: "marketing", status: "backlog", priority: "medium"}')" \
    | jq -r '"  → " + .issue.id[:8] + "  " + .issue.title'
}

echo "Seeding demo backlog into $URL ..."

post \
  "Meta descriptions · 12 new arrivals" \
  "Bulk meta description refresh across the new-arrivals collection. Each draft lands brand-voice tuned and keyphrase-led, under 155 chars. Operator typically approves with light inline edits (~4 min for the set)."

post \
  "Mother's Day campaign plan · 3 weeks out" \
  "Multi-touch campaign for the gifting collection (May 4–12). Plan covers a welcome series, landing-page copy, paid social, and a last-chance reminder. Approving the plan stages each component for In Review when drafted — approval doesn't ship anything by itself."

post \
  "Welcome email sequence · 4-email gifting series" \
  "Drafts a four-email welcome series for the gifting list: intro, product spotlight, social proof, last-chance. Operator reviews each email in order and approves to schedule. Drafts on Tue 8:30."

echo "Done."
