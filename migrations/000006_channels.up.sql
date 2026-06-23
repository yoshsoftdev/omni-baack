-- Phase 9A: channels (WhatsApp), credentials, onboarding
CREATE TABLE channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(32) NOT NULL DEFAULT 'meta',
    status channel_status NOT NULL DEFAULT 'disconnected',
    connection_type channel_connection_type,
    phone_number VARCHAR(32),
    waba_id VARCHAR(64),
    phone_number_id VARCHAR(64),
    is_coex BOOLEAN NOT NULL DEFAULT FALSE,
    last_connected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_channels_team_id ON channels (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_channels_team_status ON channels (team_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_channels_created_at ON channels (created_at DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_channels_team_phone_number_id_active ON channels (team_id, phone_number_id)
    WHERE deleted_at IS NULL AND phone_number_id IS NOT NULL;
CREATE INDEX idx_channels_waba_id ON channels (waba_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_channels_phone_number_id ON channels (phone_number_id) WHERE deleted_at IS NULL;

CREATE TABLE channel_credentials (
    channel_id UUID PRIMARY KEY REFERENCES channels (id) ON DELETE CASCADE,
    access_token_enc BYTEA NOT NULL,
    token_expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE channel_onboarding (
    channel_id UUID PRIMARY KEY REFERENCES channels (id) ON DELETE CASCADE,
    status channel_onboarding_status NOT NULL DEFAULT 'pending',
    contacts_synced_count INTEGER NOT NULL DEFAULT 0,
    history_messages_synced INTEGER NOT NULL DEFAULT 0,
    history_sync_progress SMALLINT NOT NULL DEFAULT 0,
    error_message TEXT,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_channel_onboarding_progress CHECK (history_sync_progress >= 0 AND history_sync_progress <= 100)
);
