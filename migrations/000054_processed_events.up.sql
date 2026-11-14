-- D8: consumer idempotency survives restarts and is shared by replicas.
-- Redis stays in front as a cache; this table is the source of truth.
CREATE TABLE IF NOT EXISTS processed_events (
    key TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Retention purge (events older than the JetStream MaxAge cannot be redelivered).
CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at ON processed_events (processed_at);
