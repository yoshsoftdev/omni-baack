-- Media assets: metadata registry keyed by storage_object_id (binary SoT remains in storage-service).
-- MediaAsset.ID MUST equal the storage object UUID — never generate a separate asset UUID.

CREATE TABLE media_assets (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    mime_type VARCHAR(128) NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    storage_path TEXT NOT NULL DEFAULT '',
    content_hash VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_media_assets_file_size_non_negative CHECK (file_size >= 0)
);

CREATE INDEX idx_media_assets_team_id ON media_assets (team_id);
CREATE INDEX idx_media_assets_team_created_at ON media_assets (team_id, created_at DESC);

COMMENT ON TABLE media_assets IS
    'Media Manager asset registry. id = storage_object_id; binary remains in storage-service.';
COMMENT ON COLUMN media_assets.id IS
    'Same UUID as storage-service object id (storage_object_id). Do not allocate a new UUID.';
