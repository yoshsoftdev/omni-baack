DROP INDEX IF EXISTS idx_messages_is_deleted;
DROP INDEX IF EXISTS idx_messages_reply_to_message_id;

ALTER TABLE messages
    DROP COLUMN IF EXISTS deleted_by_source;

ALTER TABLE messages
    DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE messages
    DROP COLUMN IF EXISTS is_deleted;

ALTER TABLE messages
    DROP COLUMN IF EXISTS reply_to_message_id;
