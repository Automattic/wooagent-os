-- Chain-of-identity audit log for the Policy Enforcement Point (PRD §8.4.2).
--
-- One row per pep.Invoke call regardless of outcome: passes, denials, and
-- post-MCP failures are all recorded. Append-only — the only mutation after
-- insert is setting completed_at + outcome on the matching id when the call
-- finishes (or denial_reason if a check denied).
--
-- V1 records the full chain skeleton (plan_id/task_id/step_id) but those
-- columns will mostly be empty until the orchestrator lands. cap_token_id is
-- nullable for the same reason — V1 doesn't mint cap tokens yet (Phase 3).
--
-- The args_hash column stores a sha256 of the canonical-JSON arguments so we
-- can correlate calls without persisting the arguments themselves (which may
-- contain sensitive store data). Full arguments live on the issue's proposal
-- when the call originated from approve.

CREATE TABLE audit_invocations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id         TEXT,                -- chain context, mostly empty pre-orchestrator
    task_id         TEXT,
    step_id         TEXT,
    issue_id        TEXT,                -- when the call originated from an issue
    persona         TEXT NOT NULL,       -- e.g. "marketing"
    model           TEXT,                -- LLM that produced the call, "" for operator-driven
    prompt_hash     TEXT,                -- sha256 of the prompt, "" for operator-driven
    ability         TEXT NOT NULL,       -- fully-qualified ability name
    args_hash       TEXT NOT NULL,       -- sha256 of canonical-JSON arguments
    cap_token_id    TEXT,                -- minted when allowed; null pre-Phase-3
    intent          TEXT NOT NULL,       -- read | propose | apply
    outcome         TEXT NOT NULL,       -- pending | success | mcp_error | denied
    denial_reason   TEXT,                -- non-empty iff outcome=denied
    created_at      TEXT NOT NULL,
    completed_at    TEXT
);

CREATE INDEX idx_audit_issue   ON audit_invocations(issue_id);
CREATE INDEX idx_audit_persona ON audit_invocations(persona);
CREATE INDEX idx_audit_ability ON audit_invocations(ability);
CREATE INDEX idx_audit_created ON audit_invocations(created_at);
