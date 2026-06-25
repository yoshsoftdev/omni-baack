-- Phase 12.1: reliable NATS publish for broadcast orchestration
CREATE TABLE broadcast_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_broadcast_outbox_pending ON broadcast_outbox (created_at)
    WHERE published_at IS NULL;
