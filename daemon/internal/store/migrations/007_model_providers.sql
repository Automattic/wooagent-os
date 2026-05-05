-- WooAgent OS model provider configuration.
--
-- Replaces per-persona env-var keys (ANTHROPIC_API_KEY, OPENAI_API_KEY) with
-- durable rows. Plaintext secrets never land here — secret_ref is an opaque
-- keychain handle resolved by internal/secrets at invoke time.
--
-- Schema matches ui/src/api/client.ts (commit fb98b45 shipped the UI before
-- the daemon side). Notable shape:
--   - kind ∈ {anthropic, openai, ollama}; openai_compatible deferred (the
--     design intentionally omits a "Custom OpenAI-compatible" provider in
--     onboarding; Settings → Add model picks it up post-v0.1)
--   - name is the operator-visible display name; defaults to a derived
--     value but is editable (multiple providers of the same kind)
--   - is_default marks the fleet default; exactly one row carries it (a
--     partial unique index enforces this)
--   - last_test_status uses 'untested' rather than NULL so the UI doesn't
--     have to translate a tri-state — it surfaces the literal value

CREATE TABLE model_providers (
    id                  TEXT PRIMARY KEY,        -- "mp_<ulid>"
    kind                TEXT NOT NULL,           -- anthropic | openai | ollama
    name                TEXT NOT NULL,           -- operator-visible display name
    endpoint            TEXT,                    -- NULL for anthropic/openai (use SDK default)
    default_model       TEXT NOT NULL,           -- e.g. "claude-sonnet-4-6", "llama3.2"
    secret_ref          TEXT,                    -- keychain key; NULL for kinds without API keys
    is_default          INTEGER NOT NULL DEFAULT 0,
    last_tested_at      TEXT,                    -- ISO-8601 UTC; NULL until first /test call
    last_test_status    TEXT NOT NULL DEFAULT 'untested', -- ok | failed | untested
    last_test_error     TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_model_providers_kind ON model_providers(kind);

-- Exactly one row may carry is_default=1. The handler that flips the
-- default does so in a transaction (clear current, set new) so the
-- index never sees two-defaults state.
CREATE UNIQUE INDEX idx_model_providers_default ON model_providers(is_default) WHERE is_default = 1;
