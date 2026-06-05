DROP INDEX IF EXISTS idx_messages_sent_by;
ALTER TABLE messages DROP COLUMN IF EXISTS sent_by;
