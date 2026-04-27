# WooAgent OS · Marketing prototype

A self-contained reference prototype for the marketing-agent operator flow.
Demonstrates: kanban board → reviewing proposed changes → approving the plan
→ pushing the approved variant to a live test store.

Not wired to the daemon. Talks directly to the WooCommerce REST API on
`woo-demo-store-99cc5c.mystagingwebsite.com` (or any store you point it at).

## Hosted (read-only walkthrough)

https://github.a8c.com/pages/Automattic/wooagent-os/ — gated to anyone with a
github.a8c.com login. The "Approve & apply to store" path will throw on the
hosted build (no proxy); run locally to see the live update.

## Run it locally

```bash
cd marketing-prototype
npm install
npm run dev
```

Open http://localhost:5174.

## Redeploy the hosted version

GitHub Actions isn't enabled on the Automattic GHES instance, so deploys
are manual:

```bash
cd marketing-prototype
npm run deploy
```

That builds with the right base path, force-pushes the static output to the
`gh-pages` branch, and Pages serves it within ~30s.

## Make the "apply to store" path actually update a product

1. Click **Store settings** in the top bar.
2. Paste a WooCommerce **consumer key + secret**. Generate them at
   `/wp-admin/admin.php?page=wc-settings&tab=advanced&section=keys` on the
   target store. They're stored in `localStorage` only — the dev proxy is the
   only thing that sees them.
3. Click **Test connection** → pick a published product → **Save**.
4. Open the `Handwoven Wool Throw` card from In Review.
5. Pick a variant, hit **Approve & apply to store**.

The PUT goes through `/api/woo/products/:id` (Vite proxy → `<store>/wp-json/wc/v3/products/:id`).
The card moves to Done and a snapshot toast confirms the write.

To target a different store, set `WOO_BASE` in `.env.local`:

```
WOO_BASE=https://other-store.example.com
```

## Structure

```
src/
  App.tsx                 routes + in-memory tasks state + toast bus
  screens/
    Kanban.tsx            4-column board, click to review
    ContentReview.tsx     3 variants, brand-voice/SEO panels, approve & apply
    CampaignPlanner.tsx   parent + children, approve plan
    EmailReview.tsx       single email approval
  components/
    Topbar.tsx
    SettingsDrawer.tsx    consumer key/secret + product binding
    ToastStack.tsx
    StatusBadge.tsx
  data/
    mockTasks.ts          seed marketing tasks
    types.ts
  lib/
    woo.ts                WooCommerce REST client
    settings.ts           localStorage settings
```

## Design

Visual language follows the marketing-agent mockups (Tailwind + custom CSS
variables, marketing-pink accents). Uses a light dusting of WPDS components
(`Notice`, `Spinner`) for foundational UI. Inter / JetBrains Mono / Source
Serif 4 typefaces.
