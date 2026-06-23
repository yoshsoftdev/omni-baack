CREATE TYPE conversation_status AS ENUM ('open', 'pending', 'resolved');

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    contact_id UUID NOT NULL REFERENCES contacts (id),
    device_id UUID NOT NULL REFERENCES devices (id),
    status conversation_status NOT NULL DEFAULT 'open',
    assigned_to UUID REFERENCES users (id),
    last_message_at TIMESTAMPTZ,
    last_message_preview TEXT,
    unread_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_conversations_team_contact_device
    ON conversations (team_id, contact_id, device_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_conversations_team_last_message
    ON conversations (team_id, last_message_at DESC NULLS LAST)
    WHERE deleted_at IS NULL;

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices (id),
    contact_id UUID REFERENCES contacts (id),
    wamid VARCHAR(255),
    direction VARCHAR(16) NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    content TEXT NOT NULL,
    sender_phone VARCHAR(64) NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_messages_conversation_sent ON messages (conversation_id, sent_at ASC)
    WHERE deleted_at IS NULL;

DROP TABLE IF EXISTS inbox_messages;
