-- Phase 9A: segments, rules, materialized members
CREATE TABLE segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color VARCHAR(16) NOT NULL DEFAULT '#22c55e',
    contact_count INTEGER NOT NULL DEFAULT 0,
    is_dynamic BOOLEAN NOT NULL DEFAULT TRUE,
    last_evaluated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_segments_team_id ON segments (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_segments_team_updated_at ON segments (team_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_segments_created_at ON segments (created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE segment_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id UUID NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    field segment_rule_field NOT NULL,
    operator segment_rule_operator NOT NULL,
    value VARCHAR(255) NOT NULL,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_segment_rules_segment_id ON segment_rules (segment_id);

CREATE TABLE segment_members (
    segment_id UUID NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (segment_id, contact_id)
);

CREATE INDEX idx_segment_members_contact_id ON segment_members (contact_id);
CREATE INDEX idx_segment_members_segment_id ON segment_members (segment_id);
