-- 013_issues_dedup_key.sql
--
-- Add an opt-in dedup identity to issues so the daemon can block a persona
-- from inserting a new in_review issue when one with the same logical
-- identity is already open. Set by personas via Drafted.DedupKey; NULL
-- means "no opinion" (back-compat for any persona that doesn't set it).
--
-- See docs/specs/2026-05-18-agent-proposal-dedup-design.md.

ALTER TABLE issues ADD COLUMN dedup_key TEXT;

-- Partial index: only open rows with a key participate in the lookup.
-- Keeps the index small and the dedup query a constant-time hit.
CREATE INDEX idx_issues_dedup_open
  ON issues(persona, dedup_key)
  WHERE status = 'in_review' AND dedup_key IS NOT NULL;
