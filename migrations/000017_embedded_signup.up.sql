-- Phase 11.2: Embedded Signup persistence
ALTER TABLE channels
    ADD COLUMN business_id VARCHAR(64);

CREATE INDEX idx_channels_business_id ON channels (business_id)
    WHERE deleted_at IS NULL AND business_id IS NOT NULL;

CREATE TYPE embedded_signup_status AS ENUM ('pending', 'completed', 'failed', 'expired');

CREATE TABLE embedded_signup_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    business_id VARCHAR(64),
    status embedded_signup_status NOT NULL DEFAULT 'pending',
    oauth_state VARCHAR(128) NOT NULL,
    oauth_code_hash VARCHAR(128),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_embedded_signup_oauth_state ON embedded_signup_sessions (oauth_state);
CREATE UNIQUE INDEX idx_embedded_signup_oauth_code_hash
    ON embedded_signup_sessions (oauth_code_hash)
    WHERE oauth_code_hash IS NOT NULL;
CREATE INDEX idx_embedded_signup_team_channel_created
    ON embedded_signup_sessions (team_id, channel_id, created_at DESC);
