-- WooAgent OS store connections.
--
-- Replaces the env-var-only MCP config (WOOAGENT_MCP_URL, WOOAGENT_MCP_USER,
-- WOOAGENT_MCP_APP_PASSWORD) with durable per-store rows. The pairing window
-- is carried in-row (not a separate pairings table) because there is exactly
-- one in-flight pairing per store at any time, and stores.status enforces
-- that. Device tokens never live in plaintext on disk: token_ref is an
-- opaque keychain handle resolved at use time.
--
-- Field names + status enum match ui/src/api/client.ts (the shipped UI is
-- the authoritative contract — daemon catches up). POST /v1/stores must be
-- idempotent on url: re-posting an existing url with status pairing|expired
-- |failed resets the row with a fresh code; status='paired' returns 409.

CREATE TABLE stores (
    id                  TEXT PRIMARY KEY,        -- "store_<ulid>"
    url                 TEXT NOT NULL,           -- front-page URL the operator types
    mcp_endpoint        TEXT NOT NULL,           -- derived from url at create time
    device_name         TEXT,                    -- assigned at pair time
    status              TEXT NOT NULL,           -- pairing | paired | expired | failed
    pairing_code        TEXT,                    -- present only while status='pairing'
    expires_at          TEXT,                    -- ISO-8601 UTC; valid through to status transition
    paired_at           TEXT,                    -- ISO-8601 UTC; set when status flips to paired
    failure_reason      TEXT,                    -- reason code for status='failed'
    token_ref           TEXT,                    -- keychain key once paired; never the secret itself
    last_discovered_at  TEXT,                    -- set after first successful ability discovery
    ability_count       INTEGER,                 -- denormalized snapshot for the kanban footer
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE(url)
);

CREATE INDEX idx_stores_status ON stores(status);

-- Partial index — pairing_code is only meaningful while pairing is in flight,
-- and the lookup happens during plugin polling.
CREATE INDEX idx_stores_pairing_code ON stores(pairing_code) WHERE pairing_code IS NOT NULL;
