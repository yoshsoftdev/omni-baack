-- Phase 9A: contacts
CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    phone VARCHAR(32) NOT NULL,
    email VARCHAR(255),
    notes TEXT,
    city VARCHAR(128),
    language VARCHAR(16),
    customer_type VARCHAR(64),
    source contact_source NOT NULL DEFAULT 'manual',
    marketing_status marketing_status NOT NULL DEFAULT 'unknown',
    last_activity_at TIMESTAMPTZ,
    import_job_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_contacts_team_phone_active ON contacts (team_id, phone) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_team_id ON contacts (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_team_created_at ON contacts (team_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_team_marketing_status ON contacts (team_id, marketing_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_team_last_activity ON contacts (team_id, last_activity_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_name_trgm ON contacts USING GIN (name gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_phone_trgm ON contacts USING GIN (phone gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_email_trgm ON contacts USING GIN (email gin_trgm_ops) WHERE deleted_at IS NULL AND email IS NOT NULL;
