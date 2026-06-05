CREATE TYPE broadcast_status AS ENUM (
    'draft',
    'scheduled',
    'sending',
    'completed',
    'failed',
    'cancelled'
);

CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    name VARCHAR(100) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#6366f1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_tags_team_name ON tags (team_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    name VARCHAR(255) NOT NULL,
    phone VARCHAR(32) NOT NULL,
    email VARCHAR(255),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_contacts_team_phone ON contacts (team_id, phone)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_contacts_team_name ON contacts (team_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE TABLE contact_tags (
    contact_id UUID NOT NULL REFERENCES contacts (id),
    tag_id UUID NOT NULL REFERENCES tags (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (contact_id, tag_id)
);

CREATE TABLE broadcasts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    device_id UUID NOT NULL REFERENCES devices (id),
    name VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    status broadcast_status NOT NULL DEFAULT 'draft',
    scheduled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users (id),
    total_recipients INT NOT NULL DEFAULT 0,
    sent_count INT NOT NULL DEFAULT 0,
    failed_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_broadcasts_team_status ON broadcasts (team_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_broadcasts_scheduled ON broadcasts (scheduled_at)
    WHERE deleted_at IS NULL AND status = 'scheduled';

CREATE TABLE broadcast_recipients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    broadcast_id UUID NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    contact_id UUID REFERENCES contacts (id),
    phone VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sent', 'failed')),
    wamid VARCHAR(255),
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_broadcast_recipients_broadcast ON broadcast_recipients (broadcast_id);

CREATE TABLE broadcast_tags (
    broadcast_id UUID NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id),
    PRIMARY KEY (broadcast_id, tag_id)
);
