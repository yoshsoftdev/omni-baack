ALTER TABLE devices
    ADD COLUMN waba_id VARCHAR(64),
    ADD COLUMN phone_number_id VARCHAR(64),
    ADD COLUMN access_token TEXT,
    ADD COLUMN provider VARCHAR(32) NOT NULL DEFAULT 'meta';

CREATE UNIQUE INDEX idx_devices_phone_number_id ON devices (phone_number_id)
    WHERE deleted_at IS NULL AND phone_number_id IS NOT NULL;

CREATE TABLE inbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    device_id UUID REFERENCES devices (id),
    wamid VARCHAR(255),
    sender VARCHAR(64) NOT NULL,
    recipient VARCHAR(64),
    content TEXT NOT NULL,
    direction VARCHAR(16) NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_inbox_messages_team_received ON inbox_messages (team_id, received_at DESC)
    WHERE deleted_at IS NULL;
