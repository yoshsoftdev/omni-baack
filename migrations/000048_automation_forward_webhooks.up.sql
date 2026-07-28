CREATE TABLE automation_forward_webhooks (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    url TEXT NOT NULL,
    secret TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    events JSONB NOT NULL DEFAULT '["message.received"]'::jsonb,
    channel_id UUID REFERENCES channels (id) ON DELETE SET NULL,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CONSTRAINT automation_forward_webhooks_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX idx_automation_forward_webhooks_team
    ON automation_forward_webhooks (team_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_automation_forward_webhooks_team_active
    ON automation_forward_webhooks (team_id, status)
    WHERE deleted_at IS NULL AND status = 'active';

CREATE TABLE automation_forward_deliveries (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    webhook_id TEXT NOT NULL REFERENCES automation_forward_webhooks (id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    message_id UUID,
    conversation_id UUID,
    status VARCHAR(20) NOT NULL,
    status_code INT NOT NULL DEFAULT 0,
    error_message TEXT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT automation_forward_deliveries_status_check CHECK (status IN ('success', 'failed'))
);

CREATE INDEX idx_automation_forward_deliveries_webhook
    ON automation_forward_deliveries (webhook_id, attempted_at DESC);

CREATE INDEX idx_automation_forward_deliveries_team
    ON automation_forward_deliveries (team_id, attempted_at DESC);
