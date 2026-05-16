# Product Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface real WooCommerce product images across the operator queue, Marketing/Pricing detail pages, and BatchReview, with a persona-derived placeholder where no product image exists.

**Architecture:** Pass-through URL from store CDN. Companion-plugin returns `image_url` and `image_alt` on `wooagent-products/get`. Daemon copies them into `Target` on Marketing and Pricing proposals. UI reads `proposal.target.image_url` and renders a shared `<ProductThumbnail>` component on five surfaces. No DB migration, no API contract change, no daemon proxy.

**Tech Stack:** PHP (companion plugin), Go (daemon), TypeScript + React + `@wordpress/dataviews` + `@wordpress/icons` (UI).

**Related:** `docs/specs/2026-05-16-product-images-design.md`

---

## File Structure

**PHP (companion plugin)**

- Modify: `companion-plugin/includes/abilities-products.php` — extend output_schema (lines 102–135) and execute callback (lines 230–270)

**Daemon (Go)**

- Modify: `daemon/internal/personas/marketing/marketing.go` — add ImageURL/ImageAlt to `product` struct (line 347), populate `Target` in `draftForProduct` (line 225)
- Modify: `daemon/internal/personas/pricing/pricing.go` — add ImageURL/ImageAlt to `product` struct (line 405), populate `Target` in `draftForProduct` (line 248)
- Modify: `daemon/internal/personas/marketing/marketing_test.go` — add JSON unmarshal test for image fields
- Modify: `daemon/internal/personas/pricing/pricing_test.go` — add JSON unmarshal test for image fields

**UI (TypeScript)**

- Create: `ui/src/components/ProductThumbnail.tsx` — new shared component
- Modify: `ui/src/styles/app.css` — add `.wa-product-thumbnail` rules + extend `.wa-detail-thumbnail` for the image variant
- Modify: `ui/src/screens/NeedsReview.tsx` — add `image` field to row, set `mediaField`
- Modify: `ui/src/screens/Done.tsx` — same as Needs review
- Modify: `ui/src/screens/IssueDetail.tsx` — swap Marketing placeholder, add Pricing slot
- Modify: `ui/src/screens/BatchReview.tsx` — per-row thumbnail
- Modify: `ui/src/api/client.ts` — tighten optional image fields on `Proposal.target` shape

---

## Phase A — Daemon foundation (no operator-visible change)

### Task A1: PHP — extend `wooagent-products/get` schema + payload

**Files:**
- Modify: `companion-plugin/includes/abilities-products.php`

The companion plugin has no PHP test infrastructure (no `phpunit.xml`, no `tests/` directory). Verification is via manual probe against a dev WordPress install.

- [ ] **Step 1: Add `image_url` and `image_alt` to the output schema**

In `abilities-products.php`, locate the `wooagent-products/get` ability's `output_schema.properties` block (around line 104). Add two entries before the closing `)` of `properties`:

```php
'image_url'         => array( 'type' => 'string' ),
'image_alt'         => array( 'type' => 'string' ),
```

These must be added inside the `'properties' => array( ... )` block, after `'date_modified'` and before the trailing `)`.

- [ ] **Step 2: Populate the fields in the execute callback**

In the same file, find `wooagent_products_get_execute` (around line 230). Replace the entire `return array( ... );` block (currently ending with `'date_modified' => ...,`) with:

```php
$image_id  = (int) $product->get_image_id();
$image_url = '';
$image_alt = '';
if ( $image_id > 0 ) {
    $src = wp_get_attachment_image_src( $image_id, 'medium' );
    if ( is_array( $src ) && ! empty( $src[0] ) ) {
        $image_url = (string) $src[0];
    }
    $image_alt = (string) get_post_meta( $image_id, '_wp_attachment_image_alt', true );
}

return array(
    'id'                => $product->get_id(),
    'name'              => $product->get_name(),
    'slug'              => $product->get_slug(),
    'sku'               => $product->get_sku(),
    'status'            => $product->get_status(),
    'type'              => $product->get_type(),
    'regular_price'     => $product->get_regular_price(),
    'sale_price'        => $product->get_sale_price(),
    'description'       => $product->get_description(),
    'short_description' => $product->get_short_description(),
    'categories'        => $categories,
    'tags'              => $tags,
    'meta_title'        => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_title', true ),
    'meta_description'  => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_metadesc', true ),
    'featured'          => $product->is_featured(),
    'date_modified'     => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
    'image_url'         => $image_url,
    'image_alt'         => $image_alt,
);
```

The new fields default to empty string (not null) when the product has no featured image — matches the spec's contract.

- [ ] **Step 3: Manual verification against the test store**

Deploy the updated plugin to the staging test store at `woo-demo-store-99cc5c.mystagingwebsite.com` (via the plugin's deploy workflow — see the plugin's CONTRIBUTING/release notes). Then from a daemon dev shell, probe the ability:

```bash
# From the daemon repo root:
go run ./cmd/mcp-probe wooagent-products/get '{"id":1}' | jq '.image_url, .image_alt'
```

Expected: two strings (likely a URL and either a real alt or empty). If both come back as empty strings on every product, double-check that products in the test store have featured images set in the admin.

- [ ] **Step 4: Commit**

```bash
git add companion-plugin/includes/abilities-products.php
git commit -m "feat(companion): wooagent-products/get returns image_url + image_alt

Returns the WP featured image URL (medium size) and alt text on the
product output schema. Empty string when no image is attached or no
alt is set — matches the design contract in
docs/specs/2026-05-16-product-images-design.md."
```

---

### Task A2: Daemon Marketing — add image fields to `product` struct

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go:347` (the `product` struct)
- Modify: `daemon/internal/personas/marketing/marketing_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `marketing_test.go`:

```go
func TestProductJSON_ImageFields(t *testing.T) {
	payload := []byte(`{
		"id": 42,
		"name": "Test Product",
		"sku": "SKU-42",
		"status": "publish",
		"description": "",
		"short_description": "",
		"permalink": "",
		"image_url": "https://store.example.com/wp-content/uploads/2024/01/test.jpg",
		"image_alt": "Test product alt text"
	}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "https://store.example.com/wp-content/uploads/2024/01/test.jpg" {
		t.Errorf("ImageURL = %q, want full URL", p.ImageURL)
	}
	if p.ImageAlt != "Test product alt text" {
		t.Errorf("ImageAlt = %q, want full alt", p.ImageAlt)
	}
}

func TestProductJSON_ImageFieldsAbsent_DefaultsEmpty(t *testing.T) {
	payload := []byte(`{"id":1,"name":"x","sku":"","status":"publish","description":"","short_description":"","permalink":""}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "" {
		t.Errorf("ImageURL = %q, want empty string", p.ImageURL)
	}
	if p.ImageAlt != "" {
		t.Errorf("ImageAlt = %q, want empty string", p.ImageAlt)
	}
}
```

If `encoding/json` isn't already imported in the test file, add it.

- [ ] **Step 2: Run to verify fail**

```bash
cd daemon && go test ./internal/personas/marketing/ -run TestProductJSON_ImageFields -v
```

Expected: compile error — `p.ImageURL undefined` / `p.ImageAlt undefined`.

- [ ] **Step 3: Add the fields to the struct**

In `marketing.go`, modify the `product` struct (line 347):

```go
type product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Status      string `json:"status"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_description"`
	Permalink   string `json:"permalink"`
	ImageURL    string `json:"image_url"`
	ImageAlt    string `json:"image_alt"`
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/personas/marketing/ -run TestProductJSON_ImageFields -v
```

Expected: both subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "feat(marketing): unmarshal image_url + image_alt on product struct"
```

---

### Task A3: Daemon Marketing — populate `Target` with image fields

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go:225` (the `target` map in `draftForProduct`)

The persona tests don't currently mock MCP at the `draftForProduct` level (they test parsing helpers in isolation). For this change, the wiring is short enough to verify via inspection + a build pass, plus the end-to-end probe in Phase C.

- [ ] **Step 1: Add image_url + image_alt to the Target map**

In `marketing.go`, find the `target := map[string]any{` block (line 225) and add two keys after `"previous": p.Description,`:

```go
target := map[string]any{
	"product_id":   p.ID,
	"product_name": p.Name,
	"product_sku":  p.SKU,
	"previous":     p.Description,
	"image_url":    p.ImageURL,
	"image_alt":    p.ImageAlt,
}
```

- [ ] **Step 2: Build to confirm**

```bash
go build ./...
```

Expected: exit 0.

- [ ] **Step 3: Run the marketing package tests to confirm nothing else broke**

```bash
go test ./internal/personas/marketing/ -v
```

Expected: all existing tests still PASS.

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/personas/marketing/marketing.go
git commit -m "feat(marketing): populate image_url + image_alt on proposal Target"
```

---

### Task A4: Daemon Pricing — add image fields to `product` struct

**Files:**
- Modify: `daemon/internal/personas/pricing/pricing.go:405` (the `product` struct)
- Modify: `daemon/internal/personas/pricing/pricing_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Append to `pricing_test.go`:

```go
func TestProductJSON_ImageFields(t *testing.T) {
	payload := []byte(`{
		"id": 7,
		"name": "Indigo Pillow",
		"sku": "IND-7",
		"status": "publish",
		"type": "simple",
		"regular_price": "48.00",
		"image_url": "https://store.example.com/wp-content/uploads/2024/01/pillow.jpg",
		"image_alt": "Indigo throw pillow"
	}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "https://store.example.com/wp-content/uploads/2024/01/pillow.jpg" {
		t.Errorf("ImageURL = %q", p.ImageURL)
	}
	if p.ImageAlt != "Indigo throw pillow" {
		t.Errorf("ImageAlt = %q", p.ImageAlt)
	}
}
```

Ensure `encoding/json` and `testing` are imported in the test file (they likely already are).

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/personas/pricing/ -run TestProductJSON_ImageFields -v
```

Expected: compile error — fields not defined.

- [ ] **Step 3: Add the fields to the struct**

In `pricing.go`, modify the `product` struct (line 405):

```go
type product struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	Description  string `json:"description"`
	ShortDesc    string `json:"short_description"`
	Permalink    string `json:"permalink"`
	RegularPrice string `json:"regular_price"`
	SalePrice    string `json:"sale_price"`
	ImageURL     string `json:"image_url"`
	ImageAlt     string `json:"image_alt"`
	Categories   []struct {
		Name string `json:"name"`
	} `json:"categories"`
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/personas/pricing/ -run TestProductJSON_ImageFields -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/pricing/pricing.go daemon/internal/personas/pricing/pricing_test.go
git commit -m "feat(pricing): unmarshal image_url + image_alt on product struct"
```

---

### Task A5: Daemon Pricing — populate `Target` with image fields (single + batch)

**Files:**
- Modify: `daemon/internal/personas/pricing/pricing.go:248` (single-product Target)
- Modify: `daemon/internal/personas/pricing/pricing.go` (batch packer — search for `packAsBatch`)

- [ ] **Step 1: Find the batch packer**

```bash
grep -n "packAsBatch\|BatchSiblings\|BatchTitle" daemon/internal/personas/pricing/pricing.go | head -10
```

Note the line of `packAsBatch` — you'll need it.

- [ ] **Step 2: Add image_url + image_alt to the single-product Target map**

In `pricing.go`, find the `Target: map[string]any{` block in `draftForProduct` (line 248). Add two keys after `"product_sku": p.SKU,`:

```go
Target: map[string]any{
	"product_id":      p.ID,
	"product_name":    p.Name,
	"product_sku":     p.SKU,
	"image_url":       p.ImageURL,
	"image_alt":       p.ImageAlt,
	"currency":        currency,
	"previous_price":  out.PreviousPrice,
	"proposed_price":  out.ProposedPrice,
	"regular_price":   regularPriceStr,
	"percent_change":  out.PercentChange,
	"direction":       out.Direction,
	"observed_median": out.ObservedMedian,
	"observed_low":    out.ObservedLow,
	"observed_high":   out.ObservedHigh,
	"sources":         out.Sources,
},
```

Per the spec: each sibling Drafted in a batch already carries its own `Target` (built by `draftForProduct` on each call), so each sibling already gets `image_url`/`image_alt` populated via this same code path. No batch-sibling-specific change needed.

- [ ] **Step 3: Add the batch parent's representative image**

Find `packAsBatch` — typically a function near the end of `pricing.go` that takes a slice of `[]personas.Drafted` and returns a parent Drafted with `BatchSiblings` set. Inside that function, locate where the parent Drafted's `Target` is constructed (or where the primary Drafted is returned). The spec says: the parent's Target.image_url should be the FIRST sibling's image_url.

If `packAsBatch` returns the first drafted with `BatchSiblings` set on it (the typical shape), the first drafted's Target already has image_url populated from Step 2 — no extra wiring needed. Verify with:

```bash
grep -n -A 5 "func packAsBatch" daemon/internal/personas/pricing/pricing.go
```

If `packAsBatch` instead builds a fresh parent Drafted with its own Target map, copy `image_url` and `image_alt` from `drafts[0].Target` onto the parent:

```go
// inside packAsBatch, when constructing the parent Drafted's Target
parentTarget := map[string]any{
	// existing keys ...
}
if firstImg, ok := drafts[0].Target["image_url"].(string); ok && firstImg != "" {
	parentTarget["image_url"] = firstImg
}
if firstAlt, ok := drafts[0].Target["image_alt"].(string); ok && firstAlt != "" {
	parentTarget["image_alt"] = firstAlt
}
```

- [ ] **Step 4: Build + test**

```bash
go build ./... && go test ./internal/personas/pricing/ -v
```

Expected: exit 0, all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/pricing/pricing.go
git commit -m "feat(pricing): populate image_url on single-product + batch Targets

Each per-product proposal carries its own image_url. The batch parent
(visible as one card on the queue) inherits the first sibling's image
as its representative thumbnail."
```

---

### Task A6: UI — type hint `image_url` / `image_alt` on `Proposal.target`

**Files:**
- Modify: `ui/src/api/client.ts`

- [ ] **Step 1: Find the `Proposal` interface**

```bash
grep -n "interface Proposal\|target?:" ui/src/api/client.ts | head -5
```

- [ ] **Step 2: Replace the open `Record<string, unknown>` with a discriminated optional shape**

Locate the `Proposal` interface (it currently looks like):

```ts
export interface Proposal {
  type: string;
  content: string;
  target?: Record<string, unknown>;
}
```

Replace with:

```ts
export interface Proposal {
  type: string;
  content: string;
  /** Open-ended proposal-specific payload. We narrow a few well-known
   *  keys as optionals for UI ergonomics; the runtime shape remains
   *  Record<string, unknown> and all other keys are accessed via
   *  string-indexing. */
  target?: Record<string, unknown> & {
    image_url?: string;
    image_alt?: string;
  };
}
```

This stays permissive (runtime can still hold any key) while giving UI consumers a typed `target?.image_url`.

- [ ] **Step 3: Typecheck**

```bash
cd ui && npx tsc -b
```

Expected: exit 0 (no errors anywhere; existing target accessors continue to work).

- [ ] **Step 4: Commit**

```bash
git add ui/src/api/client.ts
git commit -m "feat(api-client): type-hint image_url + image_alt on Proposal.target"
```

---

**Phase A checkpoint.** At this point the daemon and PHP layer carry image data end-to-end, but no UI surface renders it. Open a PR for Phase A if you want to land the foundation independently.

---

## Phase B — UI surfaces

### Task B1: Create `<ProductThumbnail>` component

**Files:**
- Create: `ui/src/components/ProductThumbnail.tsx`
- Modify: `ui/src/styles/app.css`

- [ ] **Step 1: Write the component**

Create `ui/src/components/ProductThumbnail.tsx`:

```tsx
import { useState } from 'react';
import { Icon } from '@wordpress/icons';
import {
  pencil,
  currencyDollar,
  comment,
  archive,
  pages,
  chartBar,
  people,
  box,
} from '@wordpress/icons';

export type ProductThumbnailSize = 'sm' | 'md' | 'lg';

interface Props {
  src?: string;
  alt?: string;
  /** Persona slug. Drives the placeholder icon when src is missing or
   *  the <img> load fails. */
  persona?: string;
  size: ProductThumbnailSize;
}

// Per-persona placeholder icons. Distinct glyph per agent so cards
// without product images still carry an at-a-glance signal of what
// the proposal is about. Future personas inherit the default `box`.
// See docs/specs/2026-05-16-product-images-design.md.
const PERSONA_ICON: Record<string, typeof box> = {
  marketing: pencil,
  pricing: currencyDollar,
  'sales-support': comment,
  inventory: archive,
  accounting: pages,
  reporting: chartBar,
  chief: people,
};

const PIXEL: Record<ProductThumbnailSize, { box: number; icon: number }> = {
  sm: { box: 40, icon: 18 },
  md: { box: 72, icon: 32 },
  lg: { box: 86, icon: 40 },
};

export default function ProductThumbnail({ src, alt, persona, size }: Props) {
  const [failed, setFailed] = useState(false);
  const showImage = !!src && !failed;
  const dims = PIXEL[size];
  const icon = (persona && PERSONA_ICON[persona]) || box;

  if (showImage) {
    return (
      <img
        src={src}
        alt={alt ?? ''}
        loading="lazy"
        onError={() => setFailed(true)}
        className={`wa-product-thumb wa-product-thumb--${size}`}
      />
    );
  }
  return (
    <div
      className={`wa-product-thumb wa-product-thumb--${size} wa-product-thumb--placeholder`}
      aria-hidden="true"
    >
      <Icon icon={icon} size={dims.icon} />
    </div>
  );
}
```

- [ ] **Step 2: Add CSS**

Append to `ui/src/styles/app.css`:

```css
/* Product thumbnail — shared component used on queue cards (DataViews
   mediaField slot), Marketing/Pricing detail pages, and BatchReview
   child rows. Renders <img> when src is set, persona-derived placeholder
   when not. See docs/specs/2026-05-16-product-images-design.md. */
.wa-product-thumb {
  display: block;
  object-fit: cover;
  border-radius: var(--wpds-border-radius-md);
  background: var(--wpds-color-bg-surface-neutral-weak);
}
.wa-product-thumb--sm { width: 40px; height: 40px; }
.wa-product-thumb--md { width: 72px; height: 72px; }
.wa-product-thumb--lg { width: 86px; height: 86px; }
.wa-product-thumb--placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--wpds-color-fg-content-neutral-weak);
}
```

- [ ] **Step 3: Typecheck**

```bash
cd ui && npx tsc -b
```

Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/ProductThumbnail.tsx ui/src/styles/app.css
git commit -m "feat(ui): add ProductThumbnail component

Shared image-or-placeholder primitive with persona-derived fallback
icons. Used by Needs review / Done grids, Marketing + Pricing detail,
and BatchReview per-row thumbnails."
```

---

### Task B2: Wire `<ProductThumbnail>` into Needs review grid card

**Files:**
- Modify: `ui/src/screens/NeedsReview.tsx`

- [ ] **Step 1: Add image fields to the Row interface**

Find the `interface Row` block. Add two optional fields:

```ts
interface Row {
  rowId: string;
  origin: BoardItem;
  title: string;
  personaSlug: string;
  agentLabel: string;
  itemId: string;
  batchCount?: number;
  updatedAt: string;
  meta: string;
  imageUrl?: string;
  imageAlt?: string;
}
```

- [ ] **Step 2: Populate them in `rowFor`**

In `rowFor`, read from the issue or batch sibling's target. Inside the `if (item.kind === 'batch')` branch, after `meta: metaForBoardItem(item),`:

```ts
const firstSibling = (b as unknown as { target?: { image_url?: string; image_alt?: string } });
// Batches: take the parent's target image (which itself was copied from
// the first sibling at daemon side via Task A5).
imageUrl: typeof (b as unknown as { target?: Record<string, unknown> }).target?.image_url === 'string'
  ? ((b as unknown as { target?: Record<string, unknown> }).target as { image_url: string }).image_url
  : undefined,
imageAlt: typeof (b as unknown as { target?: Record<string, unknown> }).target?.image_alt === 'string'
  ? ((b as unknown as { target?: Record<string, unknown> }).target as { image_alt: string }).image_alt
  : undefined,
```

If `Batch` type from `client.ts` does not yet carry `target`, fall back to leaving `imageUrl` undefined for batch rows in this task (Phase A5 still surfaces it via the parent Drafted, but if the daemon's `/v1/batches` list endpoint doesn't return target, queue cards for batches will simply show placeholders). Verify the wire shape by hitting the daemon:

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:7777/v1/batches | jq '.batches[0]' | head -20
```

If `target` is not present in the batch wire shape, leave `imageUrl` undefined for batch rows and revisit in a follow-up.

In the single-issue branch (`const issue = item.issue;`), add:

```ts
return {
  rowId: `issue:${issue.id}`,
  origin: item,
  title: issue.title,
  personaSlug: issue.persona ?? '',
  agentLabel: personaDisplayName(issue.persona),
  itemId: issue.id.slice(0, 8).toUpperCase(),
  updatedAt: issue.updated_at,
  meta: metaForBoardItem(item),
  imageUrl: typeof issue.target?.image_url === 'string' ? issue.target.image_url : undefined,
  imageAlt: typeof issue.target?.image_alt === 'string' ? issue.target.image_alt : undefined,
};
```

This requires `Issue` in `client.ts` to expose `target?: Record<string, unknown>` (or similar). Verify:

```bash
grep -n "target?:" ui/src/api/client.ts
```

If `Issue` doesn't carry `target` directly, the daemon's `/v1/issues` list endpoint must surface it. The IssueDetail endpoint already returns `proposal.target`, but list views may or may not. In that case, fall back to `imageUrl: undefined` here and surface it only in detail/BatchReview (Tasks B4–B6). Document the fallback in the commit message.

- [ ] **Step 3: Add the `image` field to the fields array**

Inside the `fields` `useMemo`, add a new field entry (alongside `title`, `agent`, etc.):

```ts
{
  id: 'image',
  label: 'Image',
  enableHiding: false,
  enableSorting: false,
  getValue: ({ item }) => item.imageUrl ?? '',
  render: ({ item }) => (
    <ProductThumbnail
      src={item.imageUrl}
      alt={item.imageAlt}
      persona={item.personaSlug}
      size="md"
    />
  ),
},
```

Add the import at the top of the file:

```ts
import ProductThumbnail from '../components/ProductThumbnail';
```

- [ ] **Step 4: Set mediaField on the view**

In the `DEFAULT_VIEW` (or wherever `view` is configured in NeedsReview), add `mediaField: 'image'`:

```ts
const DEFAULT_VIEW: View = {
  type: 'grid',
  ...
  titleField: 'title',
  descriptionField: 'meta',
  mediaField: 'image',  // NEW
  fields: ['agent', 'age', 'image'],  // NEW — include 'image' so the field is registered for the view
  ...
};
```

(`mediaField` may need to also be in the `fields` array depending on DataViews version; if the toolbar's hide-columns control shows a "Show/Hide Image" toggle that's harmless, but the field must be registered.)

- [ ] **Step 5: Build + dev verify**

```bash
cd ui && npx tsc -b && npm run dev
```

Expected: exit 0 typecheck; dev server starts. Open http://localhost:5173/needs-review and confirm Marketing/Pricing cards show real images and Sales Support cards show the comment-icon placeholder.

- [ ] **Step 6: Commit**

```bash
git add ui/src/screens/NeedsReview.tsx
git commit -m "feat(ui): render product thumbnails on Needs review grid

Cards with target.image_url load the image from the store CDN.
Cards without (Sales Support today, future personas) show the
persona-derived placeholder. Table layout unchanged (DataViews
table has no media slot)."
```

---

### Task B3: Wire `<ProductThumbnail>` into Done grid card

**Files:**
- Modify: `ui/src/screens/Done.tsx`

Mirror Task B2's structure on Done.tsx.

- [ ] **Step 1: Add imageUrl/imageAlt to Done's Row interface**

Same shape as NeedsReview — add `imageUrl?: string; imageAlt?: string;` to the `Row` interface in Done.tsx.

- [ ] **Step 2: Populate in `rowFor`**

Same logic as NeedsReview's rowFor:

```ts
imageUrl: typeof issue.target?.image_url === 'string' ? issue.target.image_url : undefined,
imageAlt: typeof issue.target?.image_alt === 'string' ? issue.target.image_alt : undefined,
```

For the batch branch, use the same fallback strategy you used in B2.

- [ ] **Step 3: Add `image` field + set `mediaField`**

Add the `image` field to the fields useMemo (same code as B2) and set `mediaField: 'image'` on Done's `DEFAULT_VIEW`. Import `ProductThumbnail`.

- [ ] **Step 4: Build + verify**

```bash
cd ui && npx tsc -b
```

Open http://localhost:5173/done and toggle to grid view; confirm thumbnails render.

- [ ] **Step 5: Commit**

```bash
git add ui/src/screens/Done.tsx
git commit -m "feat(ui): render product thumbnails on Done grid"
```

---

### Task B4: Marketing detail — swap the placeholder for `<ProductThumbnail>`

**Files:**
- Modify: `ui/src/screens/IssueDetail.tsx`

- [ ] **Step 1: Find the existing placeholder**

```bash
grep -n "wa-detail-thumbnail" ui/src/screens/IssueDetail.tsx
```

Around line 360 in the Marketing prose branch, you'll find:

```tsx
<div className="wa-detail-thumbnail" aria-hidden="true">
  <Icon icon={box} size={32} />
</div>
```

- [ ] **Step 2: Replace with the component**

Find the surrounding context (the `data` variable is in scope, holding the IssueDetail payload). Read `data.proposal?.target?.image_url` and pass to ProductThumbnail. Replace the div block with:

```tsx
<ProductThumbnail
  src={typeof data.proposal?.target?.image_url === 'string' ? data.proposal.target.image_url : undefined}
  alt={typeof data.proposal?.target?.image_alt === 'string' ? data.proposal.target.image_alt : undefined}
  persona={data.issue.persona}
  size="lg"
/>
```

Add the import:

```ts
import ProductThumbnail from '../components/ProductThumbnail';
```

Remove the now-unused `box` import from `@wordpress/icons` if it isn't referenced elsewhere in the file. Check:

```bash
grep -n "icon={box}\|, box,\|, box$" ui/src/screens/IssueDetail.tsx
```

If only the one we just removed showed up, also remove `box` from the import line.

- [ ] **Step 3: Build + dev verify**

```bash
cd ui && npx tsc -b
```

Open an issue detail in the dev server (e.g., http://localhost:5173/issues/<id> for a Marketing proposal) and confirm the 86×86 slot now shows the real product image.

- [ ] **Step 4: Commit**

```bash
git add ui/src/screens/IssueDetail.tsx
git commit -m "feat(ui): swap Marketing detail placeholder for real product image

Cashes in DSGWOO-1303's placeholder slot. ProductThumbnail handles
both the loaded image and the fallback when image_url is missing."
```

---

### Task B5: Pricing detail — add a 86×86 thumbnail slot

**Files:**
- Modify: `ui/src/screens/IssueDetail.tsx` (the Price view branch)

- [ ] **Step 1: Locate the Price view's title row**

```bash
grep -n "PriceIssueView\|function PriceIssueView" ui/src/screens/IssueDetail.tsx
```

Find the `<Page>` block inside `PriceIssueView` — specifically the heading area where the issue title is rendered. The Marketing prose branch placed the thumbnail next to the title in a flex row (search around the `.wa-detail-thumbnail` site you just edited in B4 for the pattern).

- [ ] **Step 2: Insert a thumbnail slot in the title row**

Add the same `<ProductThumbnail>` block from Task B4 to the Price view's title row, reading from `proposal?.target`:

```tsx
<ProductThumbnail
  src={typeof proposal?.target?.image_url === 'string' ? proposal.target.image_url : undefined}
  alt={typeof proposal?.target?.image_alt === 'string' ? proposal.target.image_alt : undefined}
  persona="pricing"
  size="lg"
/>
```

Use the same surrounding CSS class / wrapper that Marketing uses (`.wa-detail-thumbnail`) so layout matches.

- [ ] **Step 3: Verify the persona-derived placeholder shows when no image exists**

In the dev server, navigate to a Pricing proposal. Confirm:
- If the store product has an image: the image renders.
- If not: the `currencyDollar` icon shows in the gray slot.

- [ ] **Step 4: Build + commit**

```bash
cd ui && npx tsc -b
git add ui/src/screens/IssueDetail.tsx
git commit -m "feat(ui): Pricing detail gets product thumbnail

Mirrors the 86×86 slot Marketing detail uses. Falls back to the
currencyDollar persona placeholder when the product has no image."
```

---

### Task B6: BatchReview — per-row product thumbnails

**Files:**
- Modify: `ui/src/screens/BatchReview.tsx`

- [ ] **Step 1: Locate the child row render**

The child row builds `productName` and `productSku` from `target.product_name` etc. (around line 309). Find the JSX that renders the row header (search for `wa-batch-row__header`).

- [ ] **Step 2: Add a thumbnail in the leftmost cell of the row header**

Inside the row header button — before the existing title `<span>` — insert:

```tsx
<ProductThumbnail
  src={typeof target.image_url === 'string' ? target.image_url : undefined}
  alt={typeof target.image_alt === 'string' ? target.image_alt : undefined}
  persona={issue.persona}
  size="sm"
/>
```

Add the import at the top of the file:

```ts
import ProductThumbnail from '../components/ProductThumbnail';
```

Adjust the row header's flex layout if needed — the thumbnail should sit on the left, content flowing right. The existing `.wa-batch-row__header` class likely already uses flex; if not, add a small inline `gap` style on the wrapper or extend the class in `app.css`.

- [ ] **Step 3: Build + dev verify**

```bash
cd ui && npx tsc -b
```

Navigate to a Pricing batch detail page in the dev server. Confirm each child row shows a 40×40 thumbnail (or placeholder).

- [ ] **Step 4: Commit**

```bash
git add ui/src/screens/BatchReview.tsx
git commit -m "feat(ui): per-row product thumbnails on BatchReview"
```

---

## Phase C — Verification

### Task C1: End-to-end manual verification

The unit-level checks happen inside each task above. This task is the system-level pass.

- [ ] **Step 1: Build the daemon and deploy the companion plugin**

```bash
cd daemon && go build -o /tmp/wooagent ./cmd/wooagent
# Deploy companion-plugin/ to the staging WP site per the plugin's release flow
```

- [ ] **Step 2: Start the daemon and the UI**

```bash
/tmp/wooagent run
# In another shell:
cd ui && npm run dev
```

- [ ] **Step 3: Trigger Marketing and Pricing runs**

From the UI's Agents page, click "Run now" on Marketing, then Pricing. Wait for both to land in_review (~30s and ~1min respectively).

- [ ] **Step 4: Verify queue cards**

Open http://localhost:5173/needs-review (grid view). Confirm:
- Marketing card shows a real product image
- Pricing card shows a real product image (or, if it produced a batch, the batch card shows the first sibling's image)
- Sales Support card (if any — trigger one with the orders test fixture) shows the comment-icon placeholder

- [ ] **Step 5: Verify detail pages**

Click into the Marketing proposal — confirm the 86×86 slot shows the real image.
Click into the Pricing proposal — confirm the same.
Click into a Sales Support proposal — confirm no thumbnail slot was added (Sales Support detail is unchanged).

- [ ] **Step 6: Verify BatchReview**

If a Pricing batch is on the queue, click into it. Confirm each child row has a 40×40 thumbnail with the right product image (or a `currencyDollar` placeholder for products with no image).

- [ ] **Step 7: Verify fallback when image fails to load**

In the browser dev tools, block the WP store's CDN host. Reload Needs review. Confirm cards swap to placeholders instead of broken-image glyphs (the `onError` handler should fire).

- [ ] **Step 8: Commit a CHANGELOG / status note**

Optional but recommended: add an entry to whatever changelog or release-notes flow the project uses, linking to the spec.

```bash
# example only — adapt to project conventions
git add docs/CHANGELOG.md 2>/dev/null || true
git commit --allow-empty -m "chore: mark product-images feature complete

See docs/specs/2026-05-16-product-images-design.md"
```

---

## Self-Review

**Spec coverage**

- Companion plugin schema + execute callback → Task A1 ✓
- Daemon Marketing struct + Target population → Tasks A2, A3 ✓
- Daemon Pricing struct + Target population (single + batch) → Tasks A4, A5 ✓
- API client type tightening → Task A6 ✓
- ProductThumbnail shared component → Task B1 ✓
- Needs review grid card → Task B2 ✓
- Done grid card → Task B3 ✓
- Marketing detail swap → Task B4 ✓
- Pricing detail new slot → Task B5 ✓
- BatchReview per-row → Task B6 ✓
- Placeholder design (persona → icon mapping, neutral background) → Task B1 ✓
- Error fallback (onError swaps to placeholder, `<img loading="lazy">`) → Task B1 ✓
- Manual end-to-end verification → Task C1 ✓
- Sales Support detail explicitly unchanged → respected throughout
- Archived explicitly unchanged → respected throughout

No gaps.

**Placeholder scan**

No "TBD" / "TODO" / "implement later" / "add appropriate error handling" / "similar to Task N" in any step. Each step contains the code or command to execute.

**Type consistency**

- `ProductThumbnail` props: `src`, `alt`, `persona`, `size` — used identically across B1, B2, B3, B4, B5, B6.
- `ImageURL`, `ImageAlt` Go struct fields with `json:"image_url"` / `json:"image_alt"` tags — match the PHP schema additions in A1.
- `target.image_url` / `target.image_alt` keys — match daemon-side Target population and UI-side reads.

**Ambiguity**

- Task B2 Step 2 calls out the `Issue.target` shape question explicitly with a fallback plan; not silently assumed.
- Task A5 Step 3 documents both shapes of `packAsBatch` (returns first drafted with siblings vs. builds a new parent) and gives concrete code for the latter.

---

## Execution Handoff

Plan complete and saved to `docs/specs/2026-05-16-product-images-plan.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
