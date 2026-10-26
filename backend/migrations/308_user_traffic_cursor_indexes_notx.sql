-- Match descending keyset pagination, including tied start timestamps.
-- Retain existing indexes for older binaries; remove redundant ones only after
-- deployment-specific EXPLAIN/usage measurements and the rollback window.
CREATE INDEX CONCURRENTLY IF NOT EXISTS user_traffic_events_user_cursor_idx
    ON user_traffic_events (user_id, started_at DESC, event_id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS user_traffic_events_key_cursor_idx
    ON user_traffic_events (api_key_id, started_at DESC, event_id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS user_traffic_events_cursor_idx
    ON user_traffic_events (started_at DESC, event_id DESC);
