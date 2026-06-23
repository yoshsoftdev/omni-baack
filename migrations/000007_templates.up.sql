-- Phase 9A: Meta WhatsApp templates
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES channels (id) ON DELETE RESTRICT,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    waba_id VARCHAR(64) NOT NULL,
    meta_template_id VARCHAR(128) NOT NULL,
    name VARCHAR(512) NOT NULL,
    language VARCHAR(16) NOT NULL,
    category template_category,
    status template_status NOT NULL DEFAULT 'PENDING',
    preview TEXT,
    components JSONB NOT NULL DEFAULT '[]'::jsonb,
    variable_count SMALLINT NOT NULL DEFAULT 0,
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_templates_channel_id ON templates (channel_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_team_id ON templates (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_channel_status ON templates (channel_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_team_name ON templates (team_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_created_at ON templates (created_at DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_templates_channel_meta_id_active ON templates (channel_id, meta_template_id)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_templates_channel_name_language_active ON templates (channel_id, name, language)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_components_gin ON templates USING GIN (components);
