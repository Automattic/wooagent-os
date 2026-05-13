-- Add dismiss + archive fields to the issues table for the Dismiss UX
-- (DSGWOO-1235). The status column gains a new allowed value 'dismissed';
-- the original 001_init.sql lists status values descriptively in a comment
-- only (no CHECK constraint), so the enum doesn't need an explicit ALTER.
--
-- New columns:
--   dismiss_reason   — canonical reason key from the dismiss dialog
--                      (tone_off | wrong_product_focus | not_needed_now |
--                      write_myself | other-archive-reasons; see api/client.ts
--                      DismissReason union)
--   dismiss_comment  — optional free-text operator comment captured in the
--                      dialog's textarea
--   dismissed_at     — ISO-8601 UTC timestamp of the dismiss action; drives
--                      the 30-day TTL countdown on the Archive screen
ALTER TABLE issues ADD COLUMN dismiss_reason  TEXT;
ALTER TABLE issues ADD COLUMN dismiss_comment TEXT;
ALTER TABLE issues ADD COLUMN dismissed_at    TEXT;

-- Optional index — the Archive screen filters by status='dismissed' first
-- (covered by idx_issues_status), then sorts by dismissed_at DESC. Add the
-- per-column index now so the eventual 30-day TTL sweeper has a cheap
-- "where dismissed_at < ?" query path too.
CREATE INDEX IF NOT EXISTS idx_issues_dismissed_at ON issues(dismissed_at);
