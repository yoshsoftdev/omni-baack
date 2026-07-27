-- Sprint 3 Phase 2: Template domain additive columns.
-- Do not drop or rewrite existing template data.

ALTER TABLE templates
    ADD COLUMN IF NOT EXISTS normalized_language VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sync_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN templates.normalized_language IS
    'Stable locale key for filter/search; filled by TemplateLanguageResolver (Phase 5).';
COMMENT ON COLUMN templates.sync_version IS
    'Increments on Meta sync upsert; used for observability and conflict detection.';
COMMENT ON COLUMN templates.metadata IS
    'Internal template metadata (e.g. storage_object_id). Survives Meta sync; never sent to Graph API.';

-- Backfill normalized_language from existing language where still empty.
UPDATE templates
SET normalized_language = language
WHERE normalized_language = '' AND language <> '';

CREATE INDEX IF NOT EXISTS idx_templates_channel_normalized_language
    ON templates (channel_id, normalized_language)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_templates_metadata_gin
    ON templates USING GIN (metadata);
