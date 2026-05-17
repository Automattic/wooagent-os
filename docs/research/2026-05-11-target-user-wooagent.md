# WooAgent target user — 2026-05-11

**Supersedes:** (none — first run)
**Source inputs:** docs/research/INPUTS.md
**Evidence escalations performed:** none (offered, declined)

---

## Inputs at-a-glance

- **Stated target:** Solo operators and small teams running pre-revenue to $1M+ GMV WooCommerce stores. Multi-channel (DTC + B2B + wholesale), mature operations, often agency-supported, high operational complexity. Already using AI ad-hoc (ChatGPT, Claude) but without a system around it.
- **Hard constraints:**
  - Runs a WooCommerce store with admin access.
  - Has a local machine they can install software on (Mac/Windows/Linux).
- **Friction surfaces:**
  - LLM API key acquisition (~10 min, ~$0-20/mo) — UI provides link.
  - Or Ollama familiarity (~1hr learning curve) for the local-model path.
  - Terminal comfort for daemon install today (GUI installer is a v2 unlock).
- **V1 scope change confirmed during this run:** Pricing agent serves individual SKUs + bulk discount sweeps + pricing strategy advice (excludes B2B-tiered and dynamic/automated pricing). INPUTS.md updated to match.

---

## Step 1 — Candidate segments

| # | Segment | Behavioral one-liner | Hire being made | Why this product might fit |
|---|---|---|---|---|
| 1 | Lone writer-operator | In last 30 days, personally wrote ≥3 product descriptions, often after-hours. Owns Woo admin login. | "Stop being the bottleneck on product copy." | Marketing agent drafts product descriptions for review today. |
| 2 | Pricing-by-feel operator | In last 30 days, changed ≥1 product price manually on a hunch or after a competitor sighting; no pricing tooling beyond spreadsheets. | "Give me a second brain for pricing decisions." | Pricing agent now serves individual + bulk + strategy advice in v1. |
| 3 | Support-flooded operator | In last 30 days, personally answered ≥20 pre/post-sale customer messages; replies are inconsistent or untemplated. | "Draft my replies so I can edit and ship." | Sales Support agent drafts customer messages today. |
| 4 | AI-already-piping operator | In last 30 days, copy-pasted ChatGPT/Claude output into the Woo admin ≥1 time. Pays for ≥1 AI subscription. | "Give me a system around the AI I'm already using ad-hoc." | All three working agents replace the copy-paste loop directly. |
| 5 | Agency-supplemented operator | In last 30 days, waited >3 business days for agency-produced copy/pricing on a routine change. Has standing agency contract. | "Fill the gap between me and the agency for daily ops." | Operator (or agency) reviews drafts in-product — faster than the agency cycle. |

**Ranking (fit × reach × value-to-them × value-to-us):**

| # | Segment | Score |
|---|---|---|
| 4 | AI-already-piping | **500** |
| 1 | Lone writer-operator | 320 |
| 3 | Support-flooded | 240 |
| 2 | Pricing-by-feel | 240 (was 81 pre-scope-change) |
| 5 | Agency-supplemented | 24 |

Segment 4 wins on score; segments 1 and 3 overlap heavily with it (AI-piping is the *signal*, descriptions/replies are the *job*). The bullseye narrows segment 4 by job surface.

---

## Step 2 — ICP

### Bullseye

A solo operator (or 2-3 person team) running a pre-revenue to $1M+ GMV WooCommerce store who, **in the last 30 days, copy-pasted output from ChatGPT or Claude into the Woo admin at least once** — most often when drafting a product description, drafting a customer reply, or asking "what should I price this at / when should I run a sale?" They pay for ≥1 AI subscription, install their own plugins, and feel the friction of the AI-to-Woo handoff every time they do it. They are hiring WooAgent to **replace the copy-paste loop with a system**: drafts and pricing recommendations arrive pre-tied to the right product, order, or campaign, reviewed in one place, approved in one click. The product fits today because Marketing (descriptions), Sales Support (replies), and Pricing (individual + bulk sweeps + strategy advice) are all surfaces where this user is already manually piping AI.

### Not serving in v1

- **Pricing-by-feel operators whose pain is B2B/wholesale tiered pricing or dynamic/automated pricing** — out of scope for the Pricing agent in v1 (approval-gate principle).
- **Agency-supplemented operators** — agency displaces the merchant-review step WooAgent assumes the merchant performs; GTM unclear (sell to agency or merchant?).
- **Operators not yet using AI** — combined onboarding cost (first API key + first AI fluency) too high for v1. Re-target after GUI installer + educational content land.
- **Low-SKU-velocity DTC brands** (e.g., 4 collections/year) — daily-cadence pain that makes WooAgent indispensable isn't there.

### Onboarding cost for the bullseye

- **API key acquisition** (~10 min friction) → install UI needs one-click link to Anthropic/OpenAI console + a "what to paste back" instruction page.
- **Daemon install via terminal** → copy-paste install command minimum today; GUI installer in v2 unlocks an adjacent segment.
- **"Why doesn't it write to my store automatically?"** → bullseye expects automation; approval-gating needs framing as *"every change you'd make yourself, just pre-drafted."*
- **First-session "aha"** → surface a real Marketing or Sales Support draft within the first session — don't make them wait for tomorrow's product launch.
- **Pricing strategy advice needs evidence framing** → recommendations like "raise price on category X" must show competitor data or margin signal in the draft. Pricing decisions need more justification than copy decisions.

---

## Step 3 — Recruitable filter

### Screener (behavioral)

1. Walk me through the last time you used ChatGPT, Claude, or another AI tool to help with something for your store. What did you do with the output once you had it?
2. Roughly how many orders did your store process last month?
3. What AI tools do you currently pay for — including ChatGPT Plus or Claude Pro consumer subscriptions, **API access through an OpenAI / Anthropic developer account**, or Woo plugins with AI features built in? If you have an Ollama or other local-model setup, mention that too. *(Does not filter — surfaces whether the participant will breeze through API-key setup or hit it cold, so the moderator can recruit a deliberate mix of "has key" and "doesn't yet.")*
4. Tell me about the last product description you wrote or updated, and the last customer message you replied to. Who wrote them — you, a teammate, an agency?
5. The last time you decided to change a price or run a discount, walk me through how you made that call. What information did you look at?
6. Have you installed a WooCommerce plugin yourself in the last 6 months? Which one, and what was the install process like for you?
7. When you get stuck on a store decision — a price, a description, a customer reply — where do you go for help, if anywhere?

### Recruitment channels

1. **Twitter/X + Indie Hackers — active search for "I used ChatGPT for my Woo store" / "ChatGPT product descriptions" posts.** Highest behavioral pre-filter quality (posters *are* the bullseye). Free + DM outreach. ~1 week for 5 qualified candidates with active search. Bias skews vocal/technical, which matches the bullseye.
2. **WooCommerce community Slack + woo.com community forums.** Pre-filters for Woo + engagement (community participation correlates with self-install comfort). Free, respectful posting. 1-2 weeks. Layer screener Q1 + Q3 on top to confirm AI fluency.
3. **r/woocommerce on Reddit.** Solo-operator behavior is well-represented; apply size + AI-fluency filters via screener. Free post w/ screener link. ~2 weeks. Bias skews smaller and DIY (matches bullseye).

*Explicitly NOT:*
- **General "AI for business" Twitter/LinkedIn audiences** — severe selection bias (AI enthusiasts, consultants, influencers — not real practicing Woo operators). Screening cost per qualified lead too high.
- **r/smallbusiness on Reddit** — too generalist (cafés, freelancers, consultants); Woo + AI intersection too thin to be efficient.

---

## Open questions

- Bulk pricing + strategy advice are committed to v1, but **shipping date relative to research start** is not set. If research begins before the agent ships, reframe Q5 to probe pricing pain without promising a tested solution.
- Geographic/language scope unspecified — English-only (US/UK/AU/CA) vs. EU/non-English merchants. Affects channel choice.
- Internal-to-Automattic recruitment (existing Woo/WooPayments merchant lists) was not explored — trades external bias for internal funnel bias. Worth a separate decision before research starts.
- Pricing strategy advice was framed as needing **evidence-in-the-draft** (margin, competitor data) to be trusted — product implication, not yet validated with users.
- Evidence escalation against Wisdom/Enterpret, Linear, or Slack was offered and declined; if the AI-piping behavior signal turns out to be smaller than expected, that's the first place to check.
- **Recruitment mix ratio for API-key ownership** — what split of "already has an OpenAI/Anthropic API key" vs. "matches every other criterion but hasn't acquired a key" should the research lead recruit for? A 60/40 happy-path-vs-friction mix is a reasonable default, but the right ratio depends on whether the priority for the round is product UX (favor has-key) or onboarding friction (favor doesn't-have-key). Surfaced by screener Q3; not filtered by it.
