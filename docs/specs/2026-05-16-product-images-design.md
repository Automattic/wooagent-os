# Plumb real product images from the WooCommerce store

**Date:** 2026-05-16
**Status:** Design approved; implementation plan pending
**Related:** [DSGWOO-1303](https://linear.app/a8c/issue/DSGWOO-1303/plumb-real-product-image-url-through-daemon-api-ui-marketing-detail) (scoped narrowly to Marketing detail; this spec expands the scope)
**Branch:** `feat/product-images`

## Why

Today every proposal card on the operator's queue renders an empty gray media slot, and the Marketing detail page shows a `box`-icon placeholder where the product image should be. The operator can't tell at a glance which product a proposal is for without scanning the title. Real product images make the queue scannable and connect each proposal to the thing it's about.

The Linear ticket scopes the work narrowly to Marketing detail. This spec expands it to every surface where a proposal-with-product-context appears — and adds a designed placeholder for surfaces (Sales Support cards, future personas) where there is no single product image.

## Scope

**In scope**

- Companion-plugin (PHP) extension: `wooagent-products/get` returns `image_url` and `image_alt` (both `string`; empty string when no image is attached or no alt is set — not null).
- Daemon (Go) population: Marketing and Pricing's `draftForProduct` copy image fields into `Target`.
- UI rendering on five surfaces:
  - Needs review grid card (DataViews `mediaField`)
  - Done grid card (same)
  - Marketing detail (swap existing placeholder)
  - Pricing detail (new 86×86 slot, mirrors Marketing)
  - BatchReview per-row thumbnail
- New shared component: `ui/src/components/ProductThumbnail.tsx`.
- Persona-derived placeholder when no image URL is available, with `<img onError>` fallback.

**Out of scope**

- Sales Support detail thumbnail (proposal is about a customer message, not a single product).
- Multi-image gallery (we surface only the featured/first image).
- Daemon-side image proxy or download/cache (browser fetches directly from store CDN).
- Image rendering inside the table layout of DataViews (table has no media slot; cards-only).
- Archived page thumbnails (table-only).
- Order line-item thumbnails on Sales Support (no UI affordance per above).
- Companion-plugin extension of `wooagent-orders/get` (not needed; we are not rendering order line item images anywhere).

## Architecture

Three layers, each adds one piece of data. No API contract changes, no DB migration, no new column.

```
WooCommerce store
   │  GET /wp-json/wc/v3/products/{id} returns images[0].src + alt
   ▼
companion-plugin
   wooagent-products/get  output_schema gains:
     image_url:  string ("" when no image)
     image_alt:  string ("" when no alt)
   │
   ▼
daemon
   getProduct response struct (in marketing/marketing.go and pricing/pricing.go)
   gains ImageURL, ImageAlt fields.
   draftForProduct copies them into Target:
     Target["image_url"]  = p.ImageURL
     Target["image_alt"]  = p.ImageAlt
   Pricing batch siblings each carry per-product image_url in their own
   Target map. The PARENT Drafted (used for the queue card visual) reads
   the FIRST sibling's image_url so a batch card has a representative image.
   Sales Support: not modified.
   │
   ▼
daemon HTTP API
   No type changes — Issue.target is already Record<string, unknown> on
   the wire. image_url + image_alt appear inside the existing target JSON.
   │
   ▼
ui  client.ts
   Tighten Proposal.target typing with optional helper fields:
     image_url?: string
     image_alt?: string
   (still permissive — target stays Record<string, unknown> at runtime)
```

**Key choice — why the JSON blob, not a top-level column.** `image_url` is a property *of the proposal's target*, not of the issue itself. The same shape that already carries `product_id`, `product_name`, etc. carries this. No migration, no API type churn, and the field naturally disappears for proposals that don't have a single-product anchor (Sales Support).

**Key choice — why pass-through URL.** Stores serve images publicly with their own CDN caching headers. Proxying through the daemon would add a real proxy endpoint, an LRU cache, auth on the proxy, and disk-budget accounting for zero immediate benefit. If a store is geo-fenced or images need rewriting later, we can introduce a proxy then. YAGNI for v1.

## UI surfaces

| Surface | File | Change |
|---|---|---|
| Needs review grid card | `ui/src/screens/NeedsReview.tsx` | Add `image` field to the row; render returns `<ProductThumbnail src={item.imageUrl} alt={item.imageAlt} persona={item.personaSlug} size="md" />`. Set `mediaField: 'image'` on the view config. Table view: no change (DataViews table has no media slot). |
| Done grid card | `ui/src/screens/Done.tsx` | Same as Needs review. Grid view gets thumbnails; table view unchanged. |
| Marketing detail | `ui/src/screens/IssueDetail.tsx` (prose branch) | Existing `.wa-detail-thumbnail` div swaps its `<Icon icon={box}/>` placeholder for `<ProductThumbnail size="lg" />`. Cashes in the Linear ticket's placeholder slot. |
| Pricing detail | `ui/src/screens/IssueDetail.tsx` (Price branch) | New 86×86 slot in the same screen position as Marketing's. Uses the same `<ProductThumbnail size="lg" />`. |
| Sales Support detail | `ui/src/screens/IssueDetail.tsx` (Message branch) | No change. |
| BatchReview per-row | `ui/src/screens/BatchReview.tsx` | Each child row's leftmost column gets `<ProductThumbnail size="sm" />`. Reads each child's `target.image_url`. |
| Archived | `ui/src/screens/Archived.tsx` | No change (table-only). |

The shared component centralizes the image-or-placeholder conditional. Without it, the same branch gets copy-pasted four+ times.

### `ProductThumbnail` component

```ts
interface ProductThumbnailProps {
  src?: string;
  alt?: string;
  persona?: string;                    // for placeholder icon selection
  size: 'sm' | 'md' | 'lg';           // 40px / 72px / 86px
}
```

Behavior:
1. If `src` is set and the `<img>` loads, render `<img>` with `loading="lazy"` and `object-fit: cover`.
2. If `src` is missing OR the `<img>` `onError` fires, render the placeholder.

The placeholder is a neutral gray surface (WPDS `--wpds-color-bg-surface-neutral-weak`) with the persona-derived icon centered, color `--wpds-color-fg-content-neutral-weak`.

Icon size scales with slot: sm→18px, md→32px, lg→40px. Border-radius is `--wpds-border-radius-md` consistently.

## Placeholder — persona → icon

| Persona | Icon (`@wordpress/icons`) |
|---|---|
| `marketing` | `pencil` |
| `pricing` | `currencyDollar` |
| `sales-support` | `comment` |
| `inventory` *(future)* | `archive` |
| `accounting` *(future)* | `pages` |
| `reporting` *(future)* | `chartBar` |
| `chief` *(future)* | `people` |
| *(unknown / undefined)* | `box` |

Each persona has a distinct glyph. All eight are wired up in v1 so the four future personas have placeholders ready when those agents ship.

**Persona-color exception is not re-broadened.** Placeholders are neutral gray, not persona-tinted. The persona avatar in the card body remains the canonical persona-identity visual; the placeholder icon is a content hint, not an identity signal.

## Error handling

| Condition | Behavior |
|---|---|
| `target.image_url` is missing (Sales Support, future personas, edge cases) | Placeholder. |
| `target.image_url` is set but `<img>` fails to load (store offline, image deleted, 404) | `onError` flips local state → placeholder. Same visual. |
| Companion plugin doesn't yet have the schema upgrade (older store) | Daemon's `getProduct` unmarshals `ImageURL = ""`. Placeholder. Zero blast radius from version skew. |

Performance: queue cards fetch images on first paint. 24-card view ≈ 24 small CDN requests. `<img loading="lazy">` defers below-the-fold cards naturally; no proxy or pre-fetch needed.

## Testing

- **Companion plugin (PHP):** schema test confirms `image_url` and `image_alt` are present in `wooagent-products/get` output.
- **Daemon (Go):** unit tests for `marketing.draftForProduct` and `pricing.draftForProduct` assert `Target["image_url"]` round-trips from a mock product fixture that has an image. Existing fixtures without image data must continue to skip cleanly.
- **UI (TypeScript):** type-only checks that `ProductThumbnail` props compile at all four call sites. Visual verification is manual via dev server with the test store (`woo-demo-store-99cc5c.mystagingwebsite.com`, which has product images).
- **End-to-end (manual):** trigger Marketing and Pricing runs, confirm queue cards show real product images and Sales Support cards show the `comment` placeholder. Confirm Marketing and Pricing detail show the same image. Confirm BatchReview shows per-row thumbnails.

## Implementation order

Three phases, each independently shippable:

1. **Phase A — Foundation (PHP + Go).** Companion plugin schema, daemon `getProduct` struct + `draftForProduct` populate. Lands silently — no UI change yet, but the `image_url` flows on the wire and any new UI work can read it.
2. **Phase B — UI surfaces.** `ProductThumbnail` component, then thread through Needs review, Done, Marketing detail, Pricing detail, BatchReview. Visible work.
3. **Phase C — Cleanup.** Verify lazy-load works, confirm placeholder fallback on broken URLs, audit that no surface accidentally regressed.

Each phase is its own PR. Phase A can ship to the daemon without any operator-visible change.

## Open questions / follow-ups

None for v1. Things deliberately deferred (not in scope, not blocking):

- Multi-image gallery on detail pages
- Daemon-side proxy / cache (only needed if store CDN proves unreliable in practice)
- Pricing batch parent image policy may want refinement once we see it in the wild (currently: first sibling's image)
- Companion plugin extension to surface line-item images on `wooagent-orders/get` (for a future Sales Support detail design that reverses our "no image" decision)
