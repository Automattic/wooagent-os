-- Issues now carry the proposed change inline. The Marketing persona writes
-- a draft (e.g., a product description rewrite) into proposal_content and
-- tags it with proposal_type so the approve handler can dispatch the right
-- MCP ability when the operator clicks Approve.
--
-- Storing the proposal on the issue itself (rather than a separate proposals
-- table) keeps the v0.1 read path one query and matches the kanban surface,
-- which only ever shows one proposal per issue. A multi-proposal/variant
-- table can be added later without breaking this schema.

ALTER TABLE issues ADD COLUMN proposal_content TEXT;
ALTER TABLE issues ADD COLUMN proposal_type    TEXT;
ALTER TABLE issues ADD COLUMN proposal_target  TEXT;  -- JSON, e.g. {"product_id": 42}
