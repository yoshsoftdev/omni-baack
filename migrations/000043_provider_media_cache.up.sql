-- Provider media cache: provider-agnostic cache of external media IDs.
-- Refresh decisions use uploaded_at + MediaCachePolicy (no expires_at column).
-- state REFRESHING supports single-flight / locking in EnsureMedia (Phase 3).

CREATE TABLE provider_media_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    provider VARCHAR(32) NOT NULL,
    asset_id UUID NOT NULL REFERENCES media_assets (id) ON DELETE RESTRICT,
    phone_number_id VARCHAR(64) NOT NULL,
    provider_media_id VARCHAR(128) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    last_error TEXT,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_verified_at TIMESTAMPTZ,
    refresh_started_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_provider_media_cache_key UNIQUE (provider, asset_id, phone_number_id),
    CONSTRAINT chk_provider_media_cache_state CHECK (state IN ('ACTIVE', 'REFRESHING', 'FAILED')),
    CONSTRAINT chk_provider_media_cache_provider_nonempty CHECK (LENGTH(TRIM(provider)) > 0),
    CONSTRAINT chk_provider_media_cache_phone_nonempty CHECK (LENGTH(TRIM(phone_number_id)) > 0)
);

CREATE INDEX idx_provider_media_cache_team_id ON provider_media_cache (team_id);
CREATE INDEX idx_provider_media_cache_asset_id ON provider_media_cache (asset_id);
CREATE INDEX idx_provider_media_cache_state ON provider_media_cache (state);
CREATE INDEX idx_provider_media_cache_refreshing ON provider_media_cache (refresh_started_at)
    WHERE state = 'REFRESHING';

COMMENT ON TABLE provider_media_cache IS
    'Cached provider media IDs keyed by (provider, asset_id, phone_number_id). Binary stays in storage.';
COMMENT ON COLUMN provider_media_cache.state IS
    'ACTIVE | REFRESHING | FAILED. REFRESHING is a single-flight lock for uploads.';
COMMENT ON COLUMN provider_media_cache.refresh_started_at IS
    'Set when entering REFRESHING; used to reclaim stale locks.';
COMMENT ON COLUMN provider_media_cache.uploaded_at IS
    'Basis for MediaCachePolicy refresh age; expires_at is intentionally omitted.';
