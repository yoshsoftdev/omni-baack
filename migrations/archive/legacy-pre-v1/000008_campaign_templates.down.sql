DROP TABLE IF EXISTS campaign_audience_contacts;
DROP TABLE IF EXISTS campaign_tags;
DROP TABLE IF EXISTS campaign_contacts;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS templates;
DROP TYPE IF EXISTS campaign_contact_status;
DROP TYPE IF EXISTS campaign_status;

CREATE TYPE broadcast_status AS ENUM (
    'draft', 'scheduled', 'sending', 'completed', 'failed', 'cancelled'
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

CREATE TABLE broadcast_recipients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    broadcast_id UUID NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    contact_id UUID REFERENCES contacts (id),
    phone VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    wamid VARCHAR(255),
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE broadcast_tags (
    broadcast_id UUID NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id),
    PRIMARY KEY (broadcast_id, tag_id)
);
