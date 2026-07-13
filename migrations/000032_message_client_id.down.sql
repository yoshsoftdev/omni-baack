DROP INDEX IF EXISTS idx_messages_conversation_client_id;

ALTER TABLE messages
    DROP COLUMN IF EXISTS client_message_id;
