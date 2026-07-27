DROP INDEX IF EXISTS idx_templates_metadata_gin;
DROP INDEX IF EXISTS idx_templates_channel_normalized_language;

ALTER TABLE templates
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS sync_version,
    DROP COLUMN IF EXISTS normalized_language;
