-- DSGWOO undo support. Two columns on issues:
--   undone_at:    timestamp the operator clicked Undo on a done issue.
--                 NULL = approval still stands. Status stays 'done'.
--   applied_value: the string we actually wrote to Woo at approve time
--                 (decimal string for price changes; full description for
--                 rewrites). Undo's staleness check compares this against
--                 the live product to detect "someone else changed it
--                 after our approval" — without it, variant-selected
--                 rewrites have no comparable value to check.

ALTER TABLE issues ADD COLUMN undone_at TEXT;
ALTER TABLE issues ADD COLUMN applied_value TEXT;
