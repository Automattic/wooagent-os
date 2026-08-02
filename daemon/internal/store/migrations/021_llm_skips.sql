-- llm_skips: one upserted row per (persona, target_id) the agent examined
-- and declined at draft time (an LLM-level skip — "price already optimal",
-- "no comparable sources", "no copy to write"), distinct from an actual
-- proposal in the issues table. Read at draft time and merged into the
-- picker's skip set so a declined target drops out of contention for the
-- persona's Skipped cooldown window, then returns for re-evaluation. Bounded
-- growth (PK is (persona, target_id)); no history, no sweeper — the cooldown
-- query filters by recency and stale rows are simply ignored.
CREATE TABLE IF NOT EXISTS llm_skips (
  persona      TEXT    NOT NULL,
  target_id    INTEGER NOT NULL,
  skip_reason  TEXT,
  attempted_at TEXT    NOT NULL,            -- ISO-8601 UTC (RFC3339)
  PRIMARY KEY (persona, target_id),
  FOREIGN KEY (persona) REFERENCES agents(persona) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_llm_skips_recent ON llm_skips(persona, attempted_at);
