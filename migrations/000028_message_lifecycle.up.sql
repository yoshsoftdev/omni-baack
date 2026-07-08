ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS reply_to_message_id UUID;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS deleted_by_source VARCHAR(32);

CREATE INDEX IF NOT EXISTS idx_messages_reply_to_message_id
    ON messages (reply_to_message_id)
    WHERE reply_to_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_is_deleted
    ON messages (is_deleted, created_at DESC);
