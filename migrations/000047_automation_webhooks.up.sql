ALTER TABLE automation_endpoints
    ADD COLUMN webhook_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN webhook_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT '';

CREATE TABLE automation_webhook_deliveries (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    endpoint_id TEXT NOT NULL REFERENCES automation_endpoints (id) ON DELETE CASCADE,
    execution_id TEXT NOT NULL REFERENCES automation_executions (id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,
    status_code INT NOT NULL DEFAULT 0,
    error_message TEXT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT automation_webhook_deliveries_status_check CHECK (status IN ('success', 'failed'))
);

CREATE INDEX idx_automation_webhook_deliveries_execution
    ON automation_webhook_deliveries (execution_id, attempted_at DESC);

CREATE INDEX idx_automation_webhook_deliveries_endpoint
    ON automation_webhook_deliveries (endpoint_id, attempted_at DESC);
