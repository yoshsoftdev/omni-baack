-- Inbox priority fields for cursor-based realtime inbox ordering.

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS waiting_since TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS base_priority INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS priority_score BIGINT NOT NULL DEFAULT 0;

-- Backfill waiting_since: customer is waiting when last customer message is after last agent reply.
UPDATE conversations
SET waiting_since = last_customer_message_at
WHERE deleted_at IS NULL
  AND status IN ('open', 'pending')
  AND last_customer_message_at IS NOT NULL
  AND (last_agent_message_at IS NULL OR last_customer_message_at > last_agent_message_at);

-- Backfill base_priority from business rules.
UPDATE conversations
SET base_priority = CASE
    WHEN status IN ('closed', 'resolved') THEN -200
    ELSE LEAST(unread_count * 10, 50)
         + CASE WHEN assigned_to IS NULL AND status IN ('open', 'pending') THEN 100 ELSE 0 END
END
WHERE deleted_at IS NULL;

-- Backfill priority_score = base_priority + age_bonus(waiting_since).
UPDATE conversations
SET priority_score = base_priority + CASE
    WHEN waiting_since IS NULL THEN 0
    WHEN EXTRACT(EPOCH FROM (NOW() - waiting_since)) / 60 < 1 THEN 0
    WHEN EXTRACT(EPOCH FROM (NOW() - waiting_since)) / 60 < 2 THEN 10
    WHEN EXTRACT(EPOCH FROM (NOW() - waiting_since)) / 60 < 5 THEN 20
    WHEN EXTRACT(EPOCH FROM (NOW() - waiting_since)) / 60 < 15 THEN 50
    WHEN EXTRACT(EPOCH FROM (NOW() - waiting_since)) / 60 < 16 THEN 100
    ELSE 150
END
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_conversations_inbox_cursor
    ON conversations (team_id, priority_score DESC, last_message_at DESC NULLS LAST, id DESC)
    WHERE deleted_at IS NULL;
