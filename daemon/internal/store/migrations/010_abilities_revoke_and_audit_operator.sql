-- Trust wiring + revoke.
--
-- Adds the per-ability revoke axis (orthogonal to trust_state) and the
-- operator identity column on the audit log so operator-driven trust
-- mutations (revoke / restore / trust) carry the chain-of-identity
-- expected for launch.
--
-- The revoke axis is nullable: NULL = not revoked. The PEP treats a
-- non-NULL revoked_at as an unconditional deny that wins over manifest
-- pre-signing and over trust_state='trusted'. The operator restores by
-- nulling these columns again.
--
-- audit_invocations.operator is the auth_tokens.name that initiated
-- the action. Empty / NULL for agent-driven calls (the existing
-- pep.Invoke path); populated for operator-driven trust/revoke/restore.

ALTER TABLE abilities ADD COLUMN revoked_at TEXT;
ALTER TABLE abilities ADD COLUMN revoked_by TEXT;

ALTER TABLE audit_invocations ADD COLUMN operator TEXT;
