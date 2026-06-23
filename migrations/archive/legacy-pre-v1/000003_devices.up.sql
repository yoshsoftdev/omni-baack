CREATE TYPE device_status AS ENUM ('disconnected', 'qr_pending', 'connecting', 'connected');

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    name VARCHAR(255) NOT NULL,
    phone_number VARCHAR(32),
    status device_status NOT NULL DEFAULT 'disconnected',
    qr_code TEXT,
    last_connected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_devices_team_id ON devices (team_id) WHERE deleted_at IS NULL;
