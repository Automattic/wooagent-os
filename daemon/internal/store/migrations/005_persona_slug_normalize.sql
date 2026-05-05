-- Normalize the sales-support persona slug from `sales_support` (the
-- original Phase 1 init seed) to `sales-support` (hyphen form) so it
-- matches the manifest persona enum and the internal/personas/sales-support
-- package's Slug() value. Without this, the boot-time runPersonas guard
-- skips the persona with "not enabled in agents table" because the slug
-- it looks up doesn't match the row that exists.
--
-- Idempotent: UPDATE is a no-op when the row is already canonical, and
-- the INSERT-OR-IGNORE handles fresh DBs that never had the underscore
-- slug. Issues that FK to the old slug get repointed in the same
-- transaction so we don't leave orphans.

UPDATE issues
   SET persona = 'sales-support'
 WHERE persona = 'sales_support';

UPDATE agents
   SET persona = 'sales-support',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE persona = 'sales_support';

-- Make sure the canonical row exists on fresh DBs that skipped 001's
-- seeding for any reason. INSERT OR IGNORE leaves an existing row alone.
INSERT OR IGNORE INTO agents(persona, name, model_preference, enabled, created_at, updated_at)
VALUES (
    'sales-support',
    'Sales Support',
    'anthropic/claude-haiku-4-5-20251001',
    1,
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
);
