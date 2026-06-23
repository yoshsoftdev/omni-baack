-- Phase 9A: inbox conversations (idempotent + legacy device_id upgrade)
DO $$
BEGIN
    CREATE TYPE conversation_channel_type AS ENUM ('whatsapp', 'instagram', 'other');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE RESTRICT,
    channel_id UUID REFERENCES channels (id) ON DELETE SET NULL,
    channel_type conversation_channel_type NOT NULL DEFAULT 'whatsapp',
    status conversation_status NOT NULL DEFAULT 'open',
    assigned_to UUID REFERENCES users (id) ON DELETE SET NULL,
    unread_count INTEGER NOT NULL DEFAULT 0,
    last_message_preview TEXT NOT NULL DEFAULT '',
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Upgrade legacy conversations (device_id → channel_id)
DO $$
DECLARE
    r RECORD;
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'conversations' AND column_name = 'device_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'conversations' AND column_name = 'channel_id'
    ) THEN
        FOR r IN
            SELECT c.conname
            FROM pg_constraint c
            JOIN pg_class t ON c.conrelid = t.oid
            JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY (c.conkey)
            WHERE t.relname = 'conversations' AND a.attname = 'device_id' AND c.contype = 'f'
        LOOP
            EXECUTE format('ALTER TABLE conversations DROP CONSTRAINT %I', r.conname);
        END LOOP;

        ALTER TABLE conversations RENAME COLUMN device_id TO channel_id;
        ALTER TABLE conversations ALTER COLUMN channel_id DROP NOT NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'conversations' AND column_name = 'channel_id'
    ) THEN
        ALTER TABLE conversations ADD COLUMN channel_id UUID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'conversations_channel_id_fkey'
    ) THEN
        ALTER TABLE conversations
            ADD CONSTRAINT conversations_channel_id_fkey
            FOREIGN KEY (channel_id) REFERENCES channels (id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'conversations' AND column_name = 'channel_type'
    ) THEN
        ALTER TABLE conversations
            ADD COLUMN channel_type conversation_channel_type NOT NULL DEFAULT 'whatsapp';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'conversations' AND column_name = 'last_message_preview'
    ) THEN
        ALTER TABLE conversations
            ADD COLUMN last_message_preview TEXT NOT NULL DEFAULT '';
    ELSE
        UPDATE conversations SET last_message_preview = '' WHERE last_message_preview IS NULL;
        ALTER TABLE conversations ALTER COLUMN last_message_preview SET DEFAULT '';
        ALTER TABLE conversations ALTER COLUMN last_message_preview SET NOT NULL;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_conversations_team_contact_device;

CREATE INDEX IF NOT EXISTS idx_conversations_team_id ON conversations (team_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_team_last_message ON conversations (team_id, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_team_status_assigned ON conversations (team_id, status, assigned_to) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_contact_id ON conversations (contact_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_channel_id ON conversations (channel_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversations_team_contact_channel_active ON conversations (team_id, contact_id, channel_id)
    WHERE deleted_at IS NULL;
