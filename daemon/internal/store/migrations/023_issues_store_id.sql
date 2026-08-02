-- DSGWOO-1371 store provenance on issues.
--
-- A proposal used to record nothing about which store it was made against,
-- which made "is this proposal still relevant?" unanswerable. Re-pair to a
-- different store and every past proposal still looks current, so the Ask
-- Agent drawer happily offered chips naming products from the old catalog:
-- a store paired 2026-08-02 served Pricing chips seeded from May and June
-- proposals about a different store's products.
--
-- The first fix floored chip grounding at the paired store's paired_at. That
-- is correct but imprecise — it infers provenance from a timestamp instead of
-- recording it. This column records it.
--
-- Deliberately nullable, with no backfill. Rows created before this migration
-- genuinely have unknown provenance; writing the current store id into them
-- would be inventing data, and would wrongly promote old proposals to
-- "belongs to the store you're paired to now" — reintroducing the bug. NULL
-- means "unknown", and readers fall back to the paired_at floor for those
-- rows, which preserves today's behavior exactly.
--
-- No FK to stores(id): disconnecting a store shouldn't cascade-delete or
-- null out the history of what was proposed for it. The archive and the Done
-- list are the operator's record and should survive an unpair.

ALTER TABLE issues ADD COLUMN store_id TEXT;

-- Supports the suggestion query's `store_id = ?` filter alongside its
-- existing persona/status/created_at predicates.
CREATE INDEX IF NOT EXISTS idx_issues_store_id ON issues(store_id);
