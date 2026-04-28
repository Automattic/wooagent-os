#!/usr/bin/env bash
# Seeds the merino turtleneck demo issue into the In Review column with
# three selectable variants in proposal.target.variants[].
#
# The IssueDetail UI renders one card per variant; the operator clicks to
# pick one, and Approve sends variant_id so the daemon ships the chosen
# variant's body to WooCommerce via MCP.
#
# Usage:
#   WOOAGENT_DAEMON_TOKEN=$(wooagent auth token create | tail -1) \
#   bash daemon/scripts/seed-demo-merino-turtleneck.sh
#
# Optional:
#   WOOAGENT_DAEMON_URL  (default: http://localhost:7777)
#   PRODUCT_ID           (default: 819 — Classic Merino Wool Turtleneck on the
#                         staging store)
#   PRODUCT_SKU          (default: MWT-CL-001)

set -euo pipefail

URL="${WOOAGENT_DAEMON_URL:-http://localhost:7777}"
TOKEN="${WOOAGENT_DAEMON_TOKEN:?set WOOAGENT_DAEMON_TOKEN — try: wooagent auth token create}"
PID="${PRODUCT_ID:-819}"
SKU="${PRODUCT_SKU:-MWT-CL-001}"

PRODUCT_NAME="Classic Merino Wool Turtleneck Sweater"

PREVIOUS="Merino wool turtleneck. 100% wool. Classic fit. Available in multiple colors."

VARIANT_A_BODY="The turtleneck you'll keep. Spun from 100% extra-fine Italian merino, soft enough for next-to-skin all day, warm enough to skip the coat through fall. The ribbed collar holds its shape; cuffs and hem stretch and recover. We've worn ours for three winters — they look better the more you wear them."

VARIANT_B_BODY="100% extra-fine Italian merino wool turtleneck. 18.5-micron yarn, 14-gauge knit, ribbed collar, cuffs, and hem. Slim regular fit; 320g in size M. Machine washable on wool cycle, lay flat to dry. Available in charcoal, oat, and forest green. Sizes XS–XXL."

VARIANT_C_BODY="Italian merino. Real turtleneck. Soft, warm, lasts."

PAYLOAD=$(jq -n \
  --arg title "Product description rewrite · ${PRODUCT_NAME}" \
  --arg description "Three voice variants drafted for ${PRODUCT_NAME} (${SKU}). Pick a variant on the right; Approve writes that copy to WooCommerce." \
  --arg recommended_body "$VARIANT_A_BODY" \
  --arg variant_a_body "$VARIANT_A_BODY" \
  --arg variant_b_body "$VARIANT_B_BODY" \
  --arg variant_c_body "$VARIANT_C_BODY" \
  --arg previous "$PREVIOUS" \
  --arg sku "$SKU" \
  --arg product_name "$PRODUCT_NAME" \
  --argjson product_id "$PID" \
  '{
    title: $title,
    description: $description,
    persona: "marketing",
    status: "in_review",
    priority: "medium",
    proposal: {
      type: "product_description_rewrite",
      content: $recommended_body,
      target: {
        product_id: $product_id,
        product_name: $product_name,
        sku: $sku,
        previous: $previous,
        variants: [
          {
            id: "A",
            label: "Warm · sincere",
            body: $variant_a_body,
            seo: 88,
            voice: 96,
            charCount: ($variant_a_body | length),
            recommended: true,
            note: "Matches your last 12 approvals"
          },
          {
            id: "B",
            label: "Spec-led",
            body: $variant_b_body,
            seo: 93,
            voice: 84,
            charCount: ($variant_b_body | length),
            note: "Best for: search-heavy buyers comparing specs"
          },
          {
            id: "C",
            label: "Minimal · confident",
            body: $variant_c_body,
            seo: 64,
            voice: 92,
            charCount: ($variant_c_body | length),
            note: "Trade-off: short copy, lower SEO score · better on mobile PDPs"
          }
        ]
      }
    }
  }')

echo "Seeding merino turtleneck demo issue into $URL ..."
curl -fsS -X POST "$URL/v1/issues" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  | jq -r '"  → " + .issue.id[:8] + "  " + .issue.title + " (" + .issue.status + ")"'
echo "Done."
