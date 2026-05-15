-- Per-persona daily counters that back the PEP checkBudget gate (PRD §8.4.2
-- check 5) and the scheduler's pre-tick budget refusal.
--
-- One row per (persona, usage_date) where usage_date is the local-TZ
-- calendar date (YYYY-MM-DD). Both counters increment forward only:
--   - cost_usd: bumped when a turn_events row lands; sums model_calls'
--     cost_usd over the turn.
--   - call_count: bumped after every Allowed pep.Invoke finalization
--     (denied calls don't count — the goal is "did we touch the store").
--
-- Old rows are retained indefinitely in v1 (small data volume; ~7 personas
-- × 365 days = ~2.5K rows/year). A retention sweep can be added later.

CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,
    usage_date  TEXT NOT NULL,
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (persona, usage_date)
);

CREATE INDEX idx_persona_budget_usage_date ON persona_budget_usage(usage_date);
