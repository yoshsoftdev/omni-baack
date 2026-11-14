-- D7 / ADR-0008: the DB enforces one message per (team, wamid).
-- messages is partitioned by created_at, so its unique index (wamid, created_at)
-- cannot stop a duplicate wamid. This claim table is written in the same
-- transaction as the message insert.
CREATE TABLE IF NOT EXISTS message_wamids (
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    wamid VARCHAR(128) NOT NULL,
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, wamid)
);

-- Backfill: the earliest message wins; existing duplicates stay in messages
-- (reads already take the newest/first match), new ones are rejected.
INSERT INTO message_wamids (team_id, wamid, message_id, created_at)
SELECT DISTINCT ON (team_id, wamid) team_id, wamid, id, created_at
FROM messages
WHERE wamid IS NOT NULL AND wamid <> ''
ORDER BY team_id, wamid, created_at ASC
ON CONFLICT DO NOTHING;
