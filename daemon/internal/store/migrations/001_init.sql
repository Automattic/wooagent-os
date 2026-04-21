-- WooAgent OS initial schema.
--
-- The issue/run tables carry the v0.1 kanban surface. auth_tokens backs the
-- daemon's bearer-token HTTP auth (keychain wiring lands Phase 3). The
-- turn_events + blobs tables are the v2 GEPA telemetry substrate — schema
-- chosen now because changing it later means painful migrations across opted-in
-- operator daemons (see wooagent-os-gepa-v2-plan.md §"v1 scaffolding").

CREATE TABLE agents (
    persona             TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    model_preference    TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE issues (
    id                  TEXT PRIMARY KEY,
    title               TEXT NOT NULL,
    description         TEXT,
    persona             TEXT,
    status              TEXT NOT NULL,   -- backlog | todo | in_progress | in_review | done | rejected
    priority            TEXT NOT NULL,   -- urgent | high | medium | low | none
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    FOREIGN KEY (persona) REFERENCES agents(persona) ON DELETE SET NULL
);

CREATE INDEX idx_issues_status  ON issues(status);
CREATE INDEX idx_issues_persona ON issues(persona);

CREATE TABLE runs (
    id                  TEXT PRIMARY KEY,
    issue_id            TEXT NOT NULL,
    started_at          TEXT NOT NULL,
    completed_at        TEXT,
    status              TEXT NOT NULL,   -- running | succeeded | failed | cancelled
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX idx_runs_issue ON runs(issue_id);

-- GEPA telemetry. One row per agent turn. Large bodies live in blobs,
-- referenced by sha256. See wooagent-os-gepa-v2-plan.md §"Structured turn
-- telemetry" for schema rationale.
CREATE TABLE turn_events (
    turn_id                 TEXT PRIMARY KEY,
    event_schema_version    INTEGER NOT NULL,
    issue_id                TEXT,
    persona                 TEXT,
    prompt_version          TEXT,
    skill_versions_json     TEXT,
    started_at              TEXT NOT NULL,
    completed_at            TEXT,
    latency_ms              INTEGER,
    context_json            TEXT,
    model_calls_json        TEXT,
    skill_calls_json        TEXT,
    proposal_text           TEXT,
    proposal_sha            TEXT,
    verdict_json            TEXT,
    created_at              TEXT NOT NULL,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE SET NULL
);

CREATE INDEX idx_turn_events_issue   ON turn_events(issue_id);
CREATE INDEX idx_turn_events_persona ON turn_events(persona);

-- Content-addressed blob store. Used for request/response bodies referenced by
-- turn_events.*_sha columns; dedup across turns is automatic.
CREATE TABLE blobs (
    sha256      TEXT PRIMARY KEY,
    bytes       BLOB NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE TABLE auth_tokens (
    token_hash      TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    last_used_at    TEXT
);
