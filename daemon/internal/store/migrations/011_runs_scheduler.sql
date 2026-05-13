-- The original runs table from 001_init.sql has zero writers in the
-- codebase. Drop and rebuild for the SQLite-backed scheduler. See
-- scheduler-implementation-plan.md for the full design.

DROP TABLE IF EXISTS runs;

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    persona         TEXT NOT NULL,
    trigger         TEXT NOT NULL,                 -- tick | manual | bootstrap | retry
    status          TEXT NOT NULL,                 -- queued | running | succeeded | skipped | failed | failed_permanent
    attempt         INTEGER NOT NULL DEFAULT 1,
    retry_of        TEXT,
    scheduled_at    TEXT NOT NULL,
    claimed_at      TEXT,
    completed_at    TEXT,
    latency_ms      INTEGER,
    issue_id        TEXT,
    turn_id         TEXT,
    skip_reason     TEXT,
    failure_reason  TEXT,
    failure_class   TEXT,                          -- transient | permanent
    created_at      TEXT NOT NULL,
    FOREIGN KEY (persona)  REFERENCES agents(persona)     ON DELETE CASCADE,
    FOREIGN KEY (issue_id) REFERENCES issues(id)          ON DELETE SET NULL,
    FOREIGN KEY (turn_id)  REFERENCES turn_events(turn_id) ON DELETE SET NULL,
    FOREIGN KEY (retry_of) REFERENCES runs(id)            ON DELETE SET NULL
);

CREATE INDEX idx_runs_persona_status ON runs(persona, status);
CREATE INDEX idx_runs_scheduled      ON runs(status, scheduled_at);
CREATE INDEX idx_runs_issue          ON runs(issue_id);

ALTER TABLE agents ADD COLUMN cadence_seconds INTEGER NOT NULL DEFAULT 21600;
ALTER TABLE agents ADD COLUMN max_attempts    INTEGER NOT NULL DEFAULT 3;
ALTER TABLE agents ADD COLUMN last_run_at     TEXT;
