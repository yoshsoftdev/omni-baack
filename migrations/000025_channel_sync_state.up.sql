CREATE TABLE IF NOT EXISTS channel_sync_state (
    channel_id UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    permissions JSONB NOT NULL DEFAULT '{"contacts":false,"templates":true,"conversation_sync":false}',
    contacts_status VARCHAR(32) NOT NULL DEFAULT 'not_synced',
    contacts_created INT NOT NULL DEFAULT 0,
    contacts_updated INT NOT NULL DEFAULT 0,
    templates_status VARCHAR(32) NOT NULL DEFAULT 'not_synced',
    templates_synced INT NOT NULL DEFAULT 0,
    conversations_status VARCHAR(32) NOT NULL DEFAULT 'not_synced',
    conversations_imported INT NOT NULL DEFAULT 0,
    conversations_reason TEXT,
    contacts_sync_request_id VARCHAR(128),
    history_sync_request_id VARCHAR(128),
    last_sync_at TIMESTAMPTZ,
    contacts_last_sync_at TIMESTAMPTZ,
    templates_last_sync_at TIMESTAMPTZ,
    conversations_last_sync_at TIMESTAMPTZ,
    last_permission_check_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_sync_contact_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    action VARCHAR(32) NOT NULL,
    phone_number VARCHAR(32) NOT NULL,
    full_name VARCHAR(255),
    first_name VARCHAR(255),
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_sync_contact_events_pending
    ON channel_sync_contact_events (channel_id)
    WHERE processed_at IS NULL;
