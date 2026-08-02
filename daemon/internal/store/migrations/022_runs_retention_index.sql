-- runs retention (DSGWOO-1294): the retention sweep filters terminal runs
-- on completed_at, which was unindexed. The existing indexes from
-- 011_runs_scheduler.sql cover (persona, status), (status, scheduled_at)
-- and issue_id — none of which help a cutoff scan.
CREATE INDEX IF NOT EXISTS idx_runs_completed_at ON runs(completed_at);

-- The orphaned-turn_events pass filters on created_at for the same reason.
CREATE INDEX IF NOT EXISTS idx_turn_events_created_at ON turn_events(created_at);
