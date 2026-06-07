-- daemon_meta (activation ping, 2026-06-01): a generic singleton key/value
-- store for daemon-local state. First use: the activation ping's anonymous
-- install_id and the activation_pinged_at dedup marker.
CREATE TABLE IF NOT EXISTS daemon_meta (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
