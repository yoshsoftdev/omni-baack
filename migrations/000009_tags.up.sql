-- Phase 9A: tags and contact_tags
CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT '#6366f1',
    description TEXT,
    contact_count INTEGER NOT NULL DEFAULT 0,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_tags_team_name_active ON tags (team_id, LOWER(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_tags_team_id ON tags (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tags_created_at ON tags (created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE contact_tags (
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (contact_id, tag_id)
);

CREATE INDEX idx_contact_tags_tag_id ON contact_tags (tag_id);
CREATE INDEX idx_contact_tags_contact_id ON contact_tags (contact_id);
