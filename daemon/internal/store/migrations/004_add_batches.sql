-- Batches group sibling issues that share a generation prompt — e.g. one
-- "rewrite metadata for these 7 products" run produces seven issues, all
-- carrying the same batch_id. The operator reviews them on the batch screen
-- with a top-level counter and per-row Approve/Reject.
--
-- Lightweight grouping: status (Pending/Approved/Rejected) is DERIVED at
-- read time by joining on issues.batch_id and counting children. The batch
-- table only stores metadata (title, persona, intent, source_run_id) so we
-- can't drift out of sync with the children. See daemon-batch-issues-plan.md.

CREATE TABLE batches (
    id              TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    persona         TEXT,                -- nullable; matches issues.persona FK shape
    intent          TEXT,                -- e.g. "meta_rewrite"
    source_run_id   TEXT,                -- chain-of-identity to the run that created the batch
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (persona) REFERENCES agents(persona) ON DELETE SET NULL
);

CREATE INDEX idx_batches_created_at ON batches(created_at);

-- ON DELETE SET NULL: deleting a batch orphans its children into the
-- unbatched pool rather than cascading. V1 has no batch-delete surface so
-- this is forward-compatible only; the choice keeps issue state safe even
-- if a future delete is added.
ALTER TABLE issues ADD COLUMN batch_id TEXT REFERENCES batches(id) ON DELETE SET NULL;

CREATE INDEX idx_issues_batch_id ON issues(batch_id);
