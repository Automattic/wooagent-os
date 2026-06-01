-- persona_lessons (DSGWOO-1354): one upserted row per persona holding the
-- LLM-digested "lessons learned" block derived from recent operator
-- dismissals. Read at draft time and injected into the persona's prompt.
-- No history table — rolling window; source data ages out via the 30-day
-- dismiss sweeper. source_newest_dismissed_at doubles as the digest watermark.
CREATE TABLE IF NOT EXISTS persona_lessons (
  persona                    TEXT    PRIMARY KEY,
  lessons_text               TEXT    NOT NULL,
  generated_at               TEXT    NOT NULL,
  source_count               INTEGER NOT NULL,
  source_oldest_dismissed_at TEXT    NOT NULL,
  source_newest_dismissed_at TEXT    NOT NULL
);
