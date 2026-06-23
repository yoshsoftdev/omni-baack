-- Phase 9A: broadcast execution engine tables
CREATE TABLE broadcast_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    channel_id UUID NOT NULL REFERENCES channels (id) ON DELETE RESTRICT,
    status broadcast_job_status NOT NULL DEFAULT 'pending',
    total_recipients INTEGER NOT NULL DEFAULT 0,
    batches_total INTEGER NOT NULL DEFAULT 0,
    batches_completed INTEGER NOT NULL DEFAULT 0,
    recipients_sent INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    cancel_reason TEXT,
    error_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_broadcast_jobs_campaign_id ON broadcast_jobs (campaign_id);
CREATE INDEX idx_broadcast_jobs_team_id ON broadcast_jobs (team_id);
CREATE INDEX idx_broadcast_jobs_status ON broadcast_jobs (status);
CREATE INDEX idx_broadcast_jobs_created_at ON broadcast_jobs (created_at DESC);
CREATE UNIQUE INDEX idx_broadcast_jobs_one_active_per_campaign ON broadcast_jobs (campaign_id)
    WHERE status NOT IN ('completed', 'failed', 'cancelled');

CREATE TABLE broadcast_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES broadcast_jobs (id) ON DELETE CASCADE,
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    batch_number INTEGER NOT NULL,
    status broadcast_batch_status NOT NULL DEFAULT 'pending',
    recipient_count INTEGER NOT NULL DEFAULT 0,
    recipients_sent INTEGER NOT NULL DEFAULT 0,
    recipients_failed INTEGER NOT NULL DEFAULT 0,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    locked_by VARCHAR(128),
    locked_at TIMESTAMPTZ,
    error_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_broadcast_batches_job_number UNIQUE (job_id, batch_number)
);

CREATE INDEX idx_broadcast_batches_job_id ON broadcast_batches (job_id);
CREATE INDEX idx_broadcast_batches_campaign_id ON broadcast_batches (campaign_id);
CREATE INDEX idx_broadcast_batches_status ON broadcast_batches (status);
CREATE INDEX idx_broadcast_batches_next_retry_at ON broadcast_batches (next_retry_at)
    WHERE status IN ('queued', 'processing') AND next_retry_at IS NOT NULL;

CREATE TABLE campaign_audience_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES broadcast_jobs (id) ON DELETE CASCADE,
    snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_count INTEGER NOT NULL DEFAULT 0,
    skipped_opt_out INTEGER NOT NULL DEFAULT 0,
    skipped_invalid INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_campaign_audience_snapshots_campaign_id ON campaign_audience_snapshots (campaign_id);
CREATE INDEX idx_campaign_audience_snapshots_job_id ON campaign_audience_snapshots (job_id);

-- Partition-ready recipient ledger (monthly RANGE on created_at)
CREATE TABLE campaign_recipients (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE RESTRICT,
    job_id UUID REFERENCES broadcast_jobs (id) ON DELETE SET NULL,
    batch_id UUID REFERENCES broadcast_batches (id) ON DELETE SET NULL,
    phone VARCHAR(32) NOT NULL,
    status recipient_status NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    failure_code VARCHAR(64),
    wamid VARCHAR(128),
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at),
    CONSTRAINT uq_campaign_recipients_campaign_contact_created UNIQUE (campaign_id, contact_id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE campaign_recipients_default PARTITION OF campaign_recipients DEFAULT;

CREATE INDEX idx_campaign_recipients_campaign_id ON campaign_recipients (campaign_id);
CREATE INDEX idx_campaign_recipients_team_id ON campaign_recipients (team_id);
CREATE INDEX idx_campaign_recipients_contact_id ON campaign_recipients (contact_id);
CREATE INDEX idx_campaign_recipients_job_id ON campaign_recipients (job_id);
CREATE INDEX idx_campaign_recipients_batch_id ON campaign_recipients (batch_id);
CREATE INDEX idx_campaign_recipients_campaign_status ON campaign_recipients (campaign_id, status);
CREATE INDEX idx_campaign_recipients_team_created_at ON campaign_recipients (team_id, created_at DESC);
CREATE UNIQUE INDEX idx_campaign_recipients_wamid ON campaign_recipients (wamid, created_at)
    WHERE wamid IS NOT NULL;

-- Monotonic recipient status progression (webhook idempotency support)
CREATE OR REPLACE FUNCTION enforce_recipient_status_monotonic()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    allowed BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        RETURN NEW;
    END IF;

    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status IN ('read', 'failed', 'cancelled', 'skipped') THEN
        RAISE EXCEPTION 'recipient status cannot change from terminal state %', OLD.status;
    END IF;

    CASE OLD.status
        WHEN 'pending' THEN
            allowed := NEW.status IN ('queued', 'skipped', 'cancelled', 'failed');
        WHEN 'queued' THEN
            allowed := NEW.status IN ('sending', 'cancelled', 'failed');
        WHEN 'sending' THEN
            allowed := NEW.status IN ('sent', 'queued', 'failed', 'cancelled');
        WHEN 'sent' THEN
            allowed := NEW.status IN ('delivered', 'read', 'failed');
        WHEN 'delivered' THEN
            allowed := NEW.status IN ('read');
        ELSE
            allowed := FALSE;
    END CASE;

    IF NOT allowed THEN
        RAISE EXCEPTION 'invalid recipient status transition: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_campaign_recipients_status_monotonic
    BEFORE UPDATE OF status ON campaign_recipients
    FOR EACH ROW
    EXECUTE FUNCTION enforce_recipient_status_monotonic();
