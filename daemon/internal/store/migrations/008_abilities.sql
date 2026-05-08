-- WooAgent OS ability cache.
--
-- Each row is one ability exposed by a paired store, keyed by (store_id,
-- name). Schemas are pulled via the WP MCP Adapter's discover-abilities /
-- get-ability-info dispatcher tools and cached locally so the operator can
-- browse and trust them without re-hitting the store on every UI render.
--
-- Trust state machine (per the launch P2 "agents propose, operators approve"
-- frame):
--   new             — first time we've seen this ability for this store;
--                     no operator has approved it yet. Personas may not
--                     invoke until trusted.
--   trusted         — operator has explicitly approved this ability at the
--                     hash recorded in trusted_hash. Invocations allowed.
--   schema_changed  — discovery returned a schema_hash that differs from
--                     trusted_hash. Trust expires automatically; operator
--                     re-approves to flip back to trusted.
--
-- Hashes are SHA-256 hex of the canonicalized info JSON (sorted keys) so
-- the same logical schema produces the same hash across discoveries.
--
-- ON DELETE CASCADE: deleting a store wipes its ability cache. Trust state
-- doesn't outlive the pairing — re-pairing the same store URL is treated
-- as a fresh trust decision.

CREATE TABLE abilities (
    id              TEXT PRIMARY KEY,        -- "ab_<uuid>"
    store_id        TEXT NOT NULL,
    name            TEXT NOT NULL,           -- e.g. "wooagent-products/list"
    title           TEXT,                    -- human label, if reported
    description     TEXT,
    version         TEXT,                    -- ability semver, if reported
    schema_json     TEXT,                    -- raw JSON from get-ability-info
    schema_hash     TEXT NOT NULL,           -- sha256 of canonicalized schema_json
    trust_state     TEXT NOT NULL,           -- new | trusted | schema_changed
    trusted_hash    TEXT,                    -- schema_hash captured at last trust
    trusted_at      TEXT,                    -- ISO-8601 UTC
    last_seen_at    TEXT NOT NULL,           -- ISO-8601 UTC; touched on every successful discovery
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (store_id) REFERENCES stores(id) ON DELETE CASCADE,
    UNIQUE(store_id, name)
);

CREATE INDEX idx_abilities_store_id ON abilities(store_id);
CREATE INDEX idx_abilities_trust_state ON abilities(trust_state);
