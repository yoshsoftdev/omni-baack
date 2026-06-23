-- Phase 9A: teams (workspace) and readiness snapshots
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL,
    owner_id UUID,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Jakarta',
    country CHAR(2) NOT NULL DEFAULT 'ID',
    plan_type plan_type NOT NULL DEFAULT 'starter',
    contact_limit INTEGER NOT NULL DEFAULT 50000,
    channel_limit INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_teams_contact_limit_positive CHECK (contact_limit > 0),
    CONSTRAINT chk_teams_channel_limit_positive CHECK (channel_limit > 0)
);

CREATE UNIQUE INDEX idx_teams_slug_active ON teams (LOWER(slug)) WHERE deleted_at IS NULL;
CREATE INDEX idx_teams_plan_type ON teams (plan_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_teams_created_at ON teams (created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE workspace_readiness_snapshots (
    team_id UUID PRIMARY KEY REFERENCES teams (id) ON DELETE CASCADE,
    score SMALLINT NOT NULL DEFAULT 0,
    completed_count SMALLINT NOT NULL DEFAULT 0,
    total_count SMALLINT NOT NULL DEFAULT 5,
    readiness_status VARCHAR(20) NOT NULL DEFAULT 'not_started',
    checks JSONB NOT NULL DEFAULT '[]'::jsonb,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_workspace_readiness_score CHECK (score >= 0 AND score <= 100)
);

CREATE INDEX idx_workspace_readiness_computed_at ON workspace_readiness_snapshots (computed_at DESC);
