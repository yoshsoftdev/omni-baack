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

DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TYPE IF EXISTS conversation_status;
