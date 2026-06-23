-- Phase 9A: campaign reporting tables
CREATE TABLE campaign_link_clicks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    recipient_id UUID,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    url TEXT NOT NULL,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent TEXT,
    device_type VARCHAR(16)
);

CREATE INDEX idx_campaign_link_clicks_campaign_id ON campaign_link_clicks (campaign_id);
CREATE INDEX idx_campaign_link_clicks_campaign_clicked_at ON campaign_link_clicks (campaign_id, clicked_at);
CREATE INDEX idx_campaign_link_clicks_recipient_id ON campaign_link_clicks (recipient_id) WHERE recipient_id IS NOT NULL;

CREATE TABLE campaign_daily_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    stat_date DATE NOT NULL,
    campaigns_started INTEGER NOT NULL DEFAULT 0,
    messages_sent INTEGER NOT NULL DEFAULT 0,
    messages_delivered INTEGER NOT NULL DEFAULT 0,
    messages_read INTEGER NOT NULL DEFAULT 0,
    messages_failed INTEGER NOT NULL DEFAULT 0,
    replies_received INTEGER NOT NULL DEFAULT 0,
    link_clicks INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_daily_stats_team_date UNIQUE (team_id, stat_date)
);

CREATE INDEX idx_campaign_daily_stats_team_date ON campaign_daily_stats (team_id, stat_date DESC);
