-- DSGWOO-1282: per-persona operator-hours window for the upcoming
-- operator-hours policy.
--
-- Both columns default to NULL — the operator-hours policy is a no-op
-- unless both are set. Operators opt in by setting the columns:
--   - Today: direct UPDATE agents ... (manual)
--   - Later: via the editable-agent surface (DSGWOO-1344 et al.)
--
-- Format: "HH:MM" (24-hour, daemon-local TZ). PATCH /v1/agents validates
-- the format at write time; the policy parser re-validates as defense-
-- in-depth.

ALTER TABLE agents ADD COLUMN apply_hours_start TEXT;
ALTER TABLE agents ADD COLUMN apply_hours_end   TEXT;
