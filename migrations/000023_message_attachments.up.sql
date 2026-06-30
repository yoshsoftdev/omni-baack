-- Phase 13: message attachment references (storage object IDs only)
CREATE TABLE IF NOT EXISTS message_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    storage_object_id UUID NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    filename VARCHAR(512) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_message_attachments_message_id
    ON message_attachments (message_id);

CREATE INDEX IF NOT EXISTS idx_message_attachments_team_storage_object
    ON message_attachments (team_id, storage_object_id);
