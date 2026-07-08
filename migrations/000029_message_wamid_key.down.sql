DROP INDEX IF EXISTS idx_messages_wamid_key;

ALTER TABLE messages
    DROP COLUMN IF EXISTS wamid_key;
