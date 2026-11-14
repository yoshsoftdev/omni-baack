-- name: IsProcessed :one
SELECT EXISTS (SELECT 1 FROM processed_events WHERE key = $1);

-- name: MarkProcessed :exec
INSERT INTO processed_events (key) VALUES ($1) ON CONFLICT (key) DO NOTHING;

-- name: PurgeProcessedBefore :execrows
DELETE FROM processed_events WHERE processed_at < $1;
