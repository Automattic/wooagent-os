#!/usr/bin/env bash
# Seeds a 7-product meta-rewrite batch into the daemon — the demo for the
# "Review & approve · 7 meta rewrites" batch screen (Figma 210:28015).
#
# One POST /v1/batches creates the parent batch + seven child issues atomically
# in a single transaction. Each child carries three variants in
# proposal.target.variants[] so the operator can pick A/B/C per row before
# clicking Approve all.
#
# Usage:
#   WOOAGENT_DAEMON_TOKEN=$(wooagent auth token create | tail -1) \
#   bash daemon/scripts/seed-demo-batch-meta-rewrites.sh
#
# Optional:
#   WOOAGENT_DAEMON_URL  (default: http://localhost:7777)

set -euo pipefail

URL="${WOOAGENT_DAEMON_URL:-http://localhost:7777}"
TOKEN="${WOOAGENT_DAEMON_TOKEN:?set WOOAGENT_DAEMON_TOKEN — try: wooagent auth token create}"

# product_id | sku | name | previous-copy
PRODUCTS=(
  "1001|BC-007|Brass Candlestick|Brass candlestick. Holds one taper. Multiple sizes available."
  "1002|CP-118|Ceramic Pour-Over Coffee Dropper|Ceramic pour-over. Single cup. Easy to clean."
  "1003|WT-019|Handwoven Wool Throw - Slate|A premium wool throw blanket. Soft, luxe, and perfect for snuggling on the couch."
  "1004|LN-042|Linen Napkin Set (4)|Linen napkins. Set of four. Multiple colors."
  "1005|CB-033|Walnut Cutting Board|Walnut cutting board. Hand-finished. Use mineral oil monthly."
  "1006|CI-056|Cast Iron Skillet 10\"|Cast iron skillet. Pre-seasoned. Made in the USA."
  "1007|BT-088|Cotton Bath Towel - Sand|Cotton bath towel. Absorbent. Available in eight colors."
)

# Build the children array iteratively: each iteration appends one child JSON
# object via jq's `+ [obj]` so we end with a single well-formed array. The
# variant copy below is template-y on purpose — the seed script's job is to
# exercise the wiring, not to ship publication-ready copy.
CHILDREN=$(jq -n '[]')

for p in "${PRODUCTS[@]}"; do
  IFS='|' read -r pid sku name previous <<<"$p"

  variant_a="The $name you'll keep. We sourced these from a small-batch maker we've worked with for years — they show up well, hold up better, and look right at home in your kitchen, your dining room, wherever you put them. Get one for yourself; get one as a gift."
  variant_b="$name. Material: solid. Origin: independent maker. Care: hand wash. Compact storage. Suitable for everyday use; presentable enough for company. SKU $sku."
  variant_c="$name. Real materials. Honest design. Lasts."

  CHILDREN=$(jq -n \
    --arg title "Meta rewrite · ${name}" \
    --arg description "Three voice variants drafted for ${name} (${sku}). Pick a variant on the right; Approve writes that copy to WooCommerce." \
    --arg recommended_body "$variant_a" \
    --arg variant_a_body "$variant_a" \
    --arg variant_b_body "$variant_b" \
    --arg variant_c_body "$variant_c" \
    --arg previous "$previous" \
    --arg sku "$sku" \
    --arg product_name "$name" \
    --argjson product_id "$pid" \
    --argjson seed "$CHILDREN" \
    '$seed + [{
      title: $title,
      description: $description,
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
    }]')
done

PAYLOAD=$(jq -n \
  --arg title "Review & approve · 7 meta rewrites" \
  --arg persona "marketing" \
  --arg intent "meta_rewrite" \
  --argjson issues "$CHILDREN" \
  '{
    title: $title,
    persona: $persona,
    intent: $intent,
    issues: $issues
  }')

echo "Seeding 7-product meta-rewrites batch into $URL ..."
curl -fsS -X POST "$URL/v1/batches" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  | jq -r '"  → batch " + .batch.id[:8] + "  " + .batch.title + " (" + (.batch.total | tostring) + " children, " + (.batch.pending | tostring) + " pending)"'
echo "Done."
