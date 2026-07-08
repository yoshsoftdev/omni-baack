ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS reply_to_wamid VARCHAR(128);

CREATE INDEX IF NOT EXISTS idx_messages_reply_to_wamid
    ON messages (reply_to_wamid, created_at DESC)
    WHERE reply_to_wamid IS NOT NULL;
