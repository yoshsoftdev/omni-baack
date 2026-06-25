-- Phase 11.5: inbox lifecycle, SLA, assignment settings, presence
ALTER TYPE conversation_status ADD VALUE IF NOT EXISTS 'pending';
ALTER TYPE conversation_status ADD VALUE IF NOT EXISTS 'resolved';

UPDATE conversations SET status = 'open' WHERE status = 'assigned';

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS first_response_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_customer_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_agent_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolution_at TIMESTAMPTZ;

DO $$
BEGIN
    CREATE TYPE assignment_mode AS ENUM ('manual', 'round_robin', 'least_loaded');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS team_inbox_settings (
    team_id UUID PRIMARY KEY REFERENCES teams (id) ON DELETE CASCADE,
    assignment_mode assignment_mode NOT NULL DEFAULT 'manual',
    reset_unread_on_reply BOOLEAN NOT NULL DEFAULT FALSE,
    round_robin_cursor INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS team_member_presence (
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'offline',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_conversations_team_unread
    ON conversations (team_id, unread_count DESC)
    WHERE deleted_at IS NULL AND unread_count > 0;

CREATE INDEX IF NOT EXISTS idx_conversations_team_unassigned
    ON conversations (team_id, last_message_at DESC)
    WHERE deleted_at IS NULL AND assigned_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_conversations_team_status
    ON conversations (team_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_conversations_sla_first_response
    ON conversations (team_id, first_response_at)
    WHERE deleted_at IS NULL AND first_response_at IS NOT NULL;
