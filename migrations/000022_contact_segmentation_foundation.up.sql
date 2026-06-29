-- Phase 12.7: contact segmentation foundation
CREATE TYPE contact_field_type AS ENUM ('text', 'number', 'date', 'boolean', 'select');

ALTER TABLE contacts
    ADD COLUMN IF NOT EXISTS company VARCHAR(255),
    ADD COLUMN IF NOT EXISTS country CHAR(2),
    ADD COLUMN IF NOT EXISTS province VARCHAR(128),
    ADD COLUMN IF NOT EXISTS job_title VARCHAR(128);

CREATE TABLE contact_field_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    field_key VARCHAR(64) NOT NULL,
    label VARCHAR(128) NOT NULL,
    field_type contact_field_type NOT NULL,
    options JSONB,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_contact_field_defs_team_key
    ON contact_field_definitions (team_id, field_key)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_contact_field_defs_team_sort
    ON contact_field_definitions (team_id, sort_order)
    WHERE deleted_at IS NULL;

CREATE TABLE contact_field_values (
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    field_definition_id UUID NOT NULL REFERENCES contact_field_definitions (id) ON DELETE CASCADE,
    value JSONB NOT NULL DEFAULT 'null'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (contact_id, field_definition_id)
);

CREATE INDEX idx_contact_field_values_definition
    ON contact_field_values (field_definition_id);
