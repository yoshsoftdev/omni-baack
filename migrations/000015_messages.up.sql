-- Phase 9A: messages (partition-ready, idempotent + legacy table rename)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relname = 'messages'
          AND c.relkind = 'r'
    ) THEN
        ALTER TABLE messages RENAME TO messages_legacy;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS messages (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE RESTRICT,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    channel_id UUID REFERENCES channels (id) ON DELETE SET NULL,
    direction message_direction NOT NULL,
    sender VARCHAR(64) NOT NULL,
    recipient VARCHAR(64),
    content TEXT NOT NULL,
    content_type message_content_type NOT NULL DEFAULT 'text',
    wamid VARCHAR(128),
    status message_status,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

DO $$
BEGIN
    CREATE TABLE messages_default PARTITION OF messages DEFAULT;
EXCEPTION
    WHEN duplicate_table THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_team_id ON messages (team_id);
CREATE INDEX IF NOT EXISTS idx_messages_channel_id ON messages (channel_id);
CREATE INDEX IF NOT EXISTS idx_messages_team_received_at ON messages (team_id, received_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_wamid ON messages (wamid, created_at) WHERE wamid IS NOT NULL;
