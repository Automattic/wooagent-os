-- DSGWOO-1296: per-run aggregate cost cap.
--
-- The PEP's existing daily-per-persona BudgetGate enforces a 24-hour ceiling
-- across a persona's whole day. This adds a SECOND, finer-grained cap: one
-- persona run (a Marketing draft, a Pricing scan, a Sales Support reply)
-- must not bill more than `run_budget_cents` of model spend, or the
-- scheduler aborts it as failed_permanent. Protects operators from runaway
-- loops where a single agent re-prompts itself indefinitely.
--
-- Stored in cents (INTEGER) to keep arithmetic exact; defaulted to 1000
-- ($10) — well above today's measured per-run costs (Marketing ~$0.05,
-- Pricing ~$0.30 with web_search, Sales Support ~$0.10) so v1 demo flows
-- never trip the gate. Operators can lower it per-persona later via PATCH
-- /v1/agents once the editable-agent surface lands the cap field
-- (DSGWOO-1344 et al.).

ALTER TABLE agents ADD COLUMN run_budget_cents INTEGER NOT NULL DEFAULT 1000;
