ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS wamid_key VARCHAR(32);

CREATE INDEX IF NOT EXISTS idx_messages_wamid_key
    ON messages (team_id, conversation_id, wamid_key, created_at DESC)
    WHERE wamid_key IS NOT NULL;
