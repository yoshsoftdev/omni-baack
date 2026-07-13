DROP INDEX IF EXISTS idx_conversations_inbox_cursor;

ALTER TABLE conversations
    DROP COLUMN IF EXISTS waiting_since,
    DROP COLUMN IF EXISTS base_priority,
    DROP COLUMN IF EXISTS priority_score;
