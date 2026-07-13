ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS client_message_id UUID NULL;

-- Partitioned messages require the partition key in unique indexes; idempotency
-- is enforced in application code via FindByClientMessageID.
CREATE INDEX IF NOT EXISTS idx_messages_conversation_client_id
    ON messages (conversation_id, client_message_id, created_at DESC)
    WHERE client_message_id IS NOT NULL AND is_deleted = FALSE;
