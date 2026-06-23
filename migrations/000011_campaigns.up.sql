-- Phase 9A: campaigns and audience sources
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    channel_id UUID NOT NULL REFERENCES channels (id) ON DELETE RESTRICT,
    template_id UUID NOT NULL REFERENCES templates (id) ON DELETE RESTRICT,
    segment_id UUID REFERENCES segments (id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status campaign_status NOT NULL DEFAULT 'draft',
    variable_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
    sample_values JSONB,
    timezone VARCHAR(64),
    send_mode send_mode,
    scheduled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    total_contacts INTEGER NOT NULL DEFAULT 0,
    sent_count INTEGER NOT NULL DEFAULT 0,
    delivered_count INTEGER NOT NULL DEFAULT 0,
    read_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    reply_count INTEGER NOT NULL DEFAULT 0,
    clicked_count INTEGER NOT NULL DEFAULT 0,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_campaigns_counters_non_negative CHECK (
        total_contacts >= 0 AND sent_count >= 0 AND delivered_count >= 0
        AND read_count >= 0 AND failed_count >= 0 AND reply_count >= 0 AND clicked_count >= 0
    )
);

CREATE INDEX idx_campaigns_team_id ON campaigns (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_team_status ON campaigns (team_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_team_created_at ON campaigns (team_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_channel_id ON campaigns (channel_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_template_id ON campaigns (template_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_segment_id ON campaigns (segment_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaigns_scheduled_at ON campaigns (scheduled_at) WHERE deleted_at IS NULL AND status = 'scheduled';
CREATE INDEX idx_campaigns_status ON campaigns (status) WHERE deleted_at IS NULL;

CREATE TABLE campaign_audiences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    source_type audience_source_type NOT NULL,
    source_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_audiences_source UNIQUE (campaign_id, source_type, source_id)
);

CREATE INDEX idx_campaign_audiences_campaign_id ON campaign_audiences (campaign_id);
