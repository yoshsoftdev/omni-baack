DROP TABLE IF EXISTS broadcast_tags;
DROP TABLE IF EXISTS broadcast_recipients;
DROP TABLE IF EXISTS broadcasts;
DROP TYPE IF EXISTS broadcast_status;

CREATE TYPE campaign_status AS ENUM (
    'draft',
    'scheduled',
    'queued',
    'sending',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE campaign_contact_status AS ENUM (
    'pending',
    'sent',
    'delivered',
    'read',
    'failed'
);

CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    device_id UUID NOT NULL REFERENCES devices (id),
    waba_id VARCHAR(64) NOT NULL,
    meta_template_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    language VARCHAR(32) NOT NULL,
    category VARCHAR(64),
    status VARCHAR(32),
    components JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_templates_team_meta_lang ON templates (team_id, meta_template_id, language)
    WHERE deleted_at IS NULL;

CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    device_id UUID NOT NULL REFERENCES devices (id),
    template_id UUID NOT NULL REFERENCES templates (id),
    name VARCHAR(255) NOT NULL,
    status campaign_status NOT NULL DEFAULT 'draft',
    variable_mapping JSONB NOT NULL DEFAULT '{}',
    scheduled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users (id),
    total_contacts INT NOT NULL DEFAULT 0,
    sent_count INT NOT NULL DEFAULT 0,
    delivered_count INT NOT NULL DEFAULT 0,
    read_count INT NOT NULL DEFAULT 0,
    failed_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_campaigns_team_status ON campaigns (team_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_campaigns_scheduled ON campaigns (scheduled_at)
    WHERE deleted_at IS NULL AND status = 'scheduled';

CREATE TABLE campaign_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    contact_id UUID REFERENCES contacts (id),
    phone VARCHAR(32) NOT NULL,
    status campaign_contact_status NOT NULL DEFAULT 'pending',
    wamid VARCHAR(255),
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_campaign_contacts_campaign ON campaign_contacts (campaign_id);
CREATE INDEX idx_campaign_contacts_wamid ON campaign_contacts (wamid)
    WHERE wamid IS NOT NULL;

CREATE TABLE campaign_tags (
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id),
    PRIMARY KEY (campaign_id, tag_id)
);

CREATE TABLE campaign_audience_contacts (
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts (id),
    PRIMARY KEY (campaign_id, contact_id)
);
