DROP INDEX IF EXISTS idx_messages_reply_to_wamid;

ALTER TABLE messages
    DROP COLUMN IF EXISTS reply_to_wamid;
