# WooPayments: What High-Value Merchants Need
**Raw analysis for product leadership | April 2026**

*Sources: woomarketingp2, woomattic, wcpayoperations P2 posts (Q1–Q4 2025, Q1 2026), customer success call transcripts, competitive intelligence newsletter, payments strategy summary, Mike Monan's field feedback.*

---

## 1. Segment: Who We're Talking About

**Established merchants** ("High value merchants") — per Julia Callicrate's segmentation:

- $1M+ annual GMV, often multi-channel (DTC + B2B + wholesale)
- Mature operations, agency-supported, high operational complexity with strong technical support
- Jobs to be done: run a reliable high-volume store, implement stabily from day one, evolve the commerce stack, understand where the store may fail
- Future product home: Woo Gold

WooPayments uses $1M+ lifetime TPV as the HVM threshold for transacting merchants; the more reliable metric is LTM (last twelve months) TPV, which shows $250K+ growing — but this group still processes far less per merchant than Stripe or Mollie users do.

---

## 2. The Numbers

The gap between platform growth and WooPayments penetration is the core problem:

| Metric | Jan/Feb 2025 | Jan/Feb 2026 | Change |
|---|---|---|---|
| $1M+ active WooCommerce stores | 5,115 | 7,041 | **+38% YoY** |
| $1M+ merchants transacting on WooPayments | 607 | 536 | **-11.7%** |
| $1M+ share of WooPayments TPV | 38% | 32% | **-6pp** |
| $250K+ WooPayments merchants (lifetime TPV) | 2,806 | 2,533 | -273 (-9.7%) |
| $250K+ WooPayments merchants (LTM TPV, reliable) | 939 | 1,214 | **+29%** |

The platform is winning HVMs. WooPayments is not.

WooPayments per-merchant TPV sits at **$2.6K/month** — vs. Stripe at **$6.0K** and Mollie at **$8.2K**. This is the lowest in the ecosystem by a wide margin. Growing revenue at scale requires winning higher-volume merchants, not just more merchants.

**Churn behavior differs sharply for HVMs:**

| Status post-churn | All merchants | HVMs ($250K+ LT TPV) |
|---|---|---|
| Still on Woo | 87% | 58% |
| Went to Shopify | 3% | **19%** |
| Went offline | 5% | 11% |
| Still on Woo, using another payment plugin | 13% of those still on Woo | **43% of those still on Woo** |

When HVMs leave WooPayments and stay on Woo, their top alternatives are the WooCommerce Stripe Gateway (53% of stores) and WooCommerce PayPal Payments (44%). They're not leaving the Woo ecosystem — they're leaving WooPayments specifically.

---

## 3. What HVMs Need from WooPayments

Synthesized from Q4 2025 + Q1 2026 HVM call transcripts, Tier 1 support tickets, CS feedback, and field observations. Organized by theme, not priority.

---

### 3a. Dispute and chargeback protection that actually works

The single most emotionally acute theme in Q4 2025 calls. Merchants feel financially exposed. Disputes require significant manual effort even after preventive steps. Merchants report losing disputes they believe they should win.

> "We had trouble disputing claims when credit card details were compromised, and it's pushing us to consider switching from WooPayments."

Q1 2026 Tier 1 tickets confirm this continues at scale — dispute handling and chargeback friction are heavily represented in support tickets, even though the topic surfaced less on calls this quarter (suggesting merchants have moved past voicing it and are simply switching).

Mike Monan's field feedback: **Stripe has a higher rate of dispute resolution in the merchant's favor.** This is a direct product gap, not a perception gap.

What merchants need: a dispute process that gives them a fair shot, clear status visibility, evidence submission that doesn't require manual heroics, and outcomes that feel equitable.

---

### 3b. Financial reporting and fee transparency

This was the dominant payment theme in Q1 2026 calls — a shift from Q4's fraud focus toward quieter structural gaps.

> "I'm missing a little bit the transparency and an overview about all the transactions, the included fees, and the receipt for the fees."

> "If you could put this down on a piece of paper or send us some PDF... pricing will be the primary criteria."

Mike Monan: Stripe has a more robust dashboard and analytics — businesses have built their reporting practices and compliance workflows around it. Switching away from Stripe means rebuilding those workflows.

Specific documented gaps:
- **Monthly ending balance reporting** (PAYOPS-87, WOOPMNT-5386) — merchants need end-of-month snapshots to reconcile accounts. Stripe provides this; WooPayments does not.
- **Summary reporting for payouts/deposits** (WOOPMNT-5386) — insufficient granularity for HVM accounting needs.

Xero Shoes (churned to Shopify): on WooCommerce they paid for Metorik plus specialists; on Shopify, "anyone types a natural-language question and gets the answer in 2 minutes."

What merchants need: a payments analytics experience that supports real business operations — reconciliation, fee visibility per transaction, monthly balance snapshots, and payout reporting at the level of detail HVMs require for accounting and compliance.

---

### 3c. Pricing competitiveness and rate transparency

HVMs are comparing WooPayments rates directly against alternatives and, at scale, the differences are material.

ScandiKitchen (~$3M TPV): was evaluating Revolut at **0.50% + £0.20** on domestic cards vs. WooPayments' **0.91% + £0.20**. That's a 40bps gap — at $3M TPV, meaningful. They retained via relationship sell, not product.

Q4 2025 calls: a merchant switched from Stripe to Clover because it was "half the price on transactions," even though Clover was harder to use. Cost is an active decision driver, not background noise.

Mike Monan: Stripe has the ability to offer better merchant rates to HVMs.

What merchants need: competitive rates at high volume, and the ability to understand exactly what they're paying and why — before they start evaluating alternatives.

---

### 3d. True multi-currency and international payment support

Q4 2025: "trying to optimize our WooCommerce store for international sales… the complexities with currency conversion and the smoothness of the checkout process have been problematic."

Mike Monan's direct assessment: **WooPayments multi-currency is cosmetic.** The Stripe offering ("adaptive pricing") shows AND pays in the buyer's local currency in 150 countries — the settlement happens in local currency behind the scenes. WooPayments multi-currency affects display but not the underlying settlement and conversion experience.

WooPayments does support payouts in multiple currencies, but the checkout-side experience for international buyers falls short of what Stripe and other modern gateways provide.

What merchants need: checkout experiences that feel native to international buyers, with real currency settlement — not just display conversion.

---

### 3e. Dynamic payment method optimization

Mike Monan: Stripe automatically selects dynamic payment methods behind the scenes based on buyer profile — driving measurable conversion increases. WooPayments does not do this.

Illustrative from the positive side: Straight To Hell Apparel ($4M GMV) discovered WooPayments included Apple Pay and Google Pay — which he had been trying to enable for years — during a support call. That integration now drives 560 Apple Pay/Google Pay orders vs. 390 credit card orders per month. The feature existed but was invisible. That's a different problem (discoverability) but points to the same gap: HVMs need optimized payment method presentation to maximize conversion.

What merchants need: payment method selection that adapts to buyer context and surfaces the highest-converting options without requiring merchant configuration.

---

### 3f. Feature parity and roadmap visibility

Mike Monan: Stripe has a **published roadmap** and first-adopts new features. WooPayments gets features "eventually." This matters to HVMs who are building long-term payment infrastructure decisions — choosing a gateway is a multi-year commitment, and merchants want confidence in the trajectory.

The WooPayments strategy doc acknowledges this structurally: the current architecture is "a white-labeled proxy over Stripe" that lacks data visibility and processor independence. HVMs sense this, even if they don't articulate it in those terms.

What merchants need: confidence that WooPayments is investing ahead of their needs, not catching up to Stripe's last release.

---

### 3g. Migration path that doesn't require starting over

Q4 2025: merchants constrained by legacy payment setups — outdated plugins introducing fraud risk — but migration to a modern gateway "feels daunting due to concerns around data access, reporting continuity, or the perceived complexity of switching."

> "Fraudulent orders have been a significant challenge… switching to a modern payment gateway, which could improve fraud protection and performance, faces resistance due to perceived difficulties in transition and concerns about payment history access."

This is a retention and acquisition problem in one. Merchants who should move to WooPayments don't, because the cost of migrating their payment history and existing customer billing relationships feels too high.

What merchants need: a migration path that preserves payment history, doesn't break existing subscriptions, and gives them visibility into what the transition will actually look like.

---

### 3h. Subscription payment reliability (emerging Q1 2026 signal)

New in Q1 2026, surfaced most clearly in Tier 1 tickets:
- Renewals moving to On Hold after successful payment
- Subscription status not updating post-payment
- Domain migration silently breaking renewals
- No recovery path when admin accidentally expires a subscription

At HVM scale, one product change put 1,300 subscriptions on Hold with no bulk recovery path. Silent billing failures that cause financial loss are discovered only after damage is done.

What merchants need: WooPayments + WooCommerce Subscriptions to work reliably at scale, with observable failure states and recovery paths — not silent failures in live money flows.

---

## 4. Competitive Context

### Where Stripe wins

Stripe advantages identified through field feedback and competitive analysis:

| Gap | Stripe | WooPayments |
|---|---|---|
| Dynamic payment methods | Buyer-profile-based automatic selection | Not available |
| Adaptive pricing | True local currency settlement in 150 countries | Cosmetic multi-currency display |
| Dispute resolution | Higher merchant-win rate | Lower merchant-win rate |
| Dashboard / analytics | Business-grade, reportable | Limited; reconciliation and balance reporting gaps |
| Roadmap transparency | Published, predictable | Not public |
| Rates (HVM) | Negotiable at scale | Less flexible |
| Brand trust | Established, "safe" for boards and procurement | Less familiar outside WP ecosystem |

### Where WooPayments can win (observed from retention data)

- **Ecosystem integration**: WooPayments as the default, pre-configured option is a genuine advantage — when merchants discover capabilities they didn't know existed (e.g., Apple Pay/Google Pay), the stickiness is real.
- **Relationship value at HVM tier**: ScandiKitchen retention via CS relationship, not pricing. HVMs respond to being known and supported.
- **Open-source architecture advantage**: As agentic commerce and AI integrations become baseline expectations, WooCommerce's architecture provides a structural flexibility advantage over Shopify Plus.

### Why Shopify is the primary HVM churn destination

19% of churned WooPayments HVMs go to Shopify — not primarily because of payments, but because of platform-level operational simplicity. Xero Shoes: admin speed (28s vs 3s on Shopify), no server management around marketing email sends, reporting without specialist cost. Shopify's payments are adequate for their needs; WooCommerce's operational tax is what pushed them out.

This matters for WooPayments: payments gaps contribute to HVM churn, but they're rarely the only factor. The "switch to Shopify" decision reflects accumulated friction across multiple domains.

---

## 5. What We Still Don't Know

These are the open questions as of Q2 2026:

1. **Primary churn driver**: Is the WooPayments HVM decline caused by active gateway switching, volume decline moving merchants out of the HVM bracket, or both? The AEDP-612/AEDP-685 investigations are ongoing.

2. **Which gaps are determinative**: Among fraud protection, reporting, rates, multi-currency, and feature parity — which gap, if closed, would most materially affect HVM retention? We have signal quality but no ranking.

3. **Stripe extension users**: The majority of churned HVMs staying on Woo switch to the WooCommerce Stripe Gateway. Are these merchants who would have preferred WooPayments if specific gaps were addressed, or do they have a Stripe preference independent of WooPayments' quality?

4. **Pre-churn evaluation losses**: Win/loss data shows a 5:1 TPV loss ratio in Q1 ($104.8M lost vs $23.2M won). How much of that is payment-related vs. platform-related?

5. **Price sensitivity at the HVM tier**: The ScandiKitchen case shows a 40bps gap triggered evaluation. What's the real rate threshold for the $1M+ cohort?

---

## Sources

- woomattic.wordpress.com/2026/04/19/high-value-merchant-call-and-ticket-themes-q1-2026/
- woomattic.wordpress.com/2026/01/16/high-value-merchant-call-themes-q4-2025/
- woomattic.wordpress.com/2026/03/04/summary-woo-payments-strategy-2026-2027/
- woomarketingp2.wordpress.com/2026/03/03/woopayments-and-high-value-merchants/
- woomarketingp2.wordpress.com/2026/03/18/woopayments-churn-path/
- woomarketingp2.wordpress.com/2026/03/12/woos-merchant-segments/
- woomarketingp2.wordpress.com/2026/04/06/woocommerce-competitive-intelligence-newsletter-jan-mar-2026/
- woomarketingp2.wordpress.com/2026/03/11/payments-product-marketing-monthly-update-mar-2026/
- Mike Monan field feedback (provided in brief)
- wcpayoperations linear tickets: PAYOPS-87, WOOPMNT-5386 (balance/payout reporting gaps)
