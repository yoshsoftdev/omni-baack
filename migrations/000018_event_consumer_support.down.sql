ALTER TABLE channels DROP COLUMN IF EXISTS health_metadata;
ALTER TABLE messages DROP COLUMN IF EXISTS raw_payload;
-- contact_source enum value meta_inbound is not removed (PostgreSQL limitation)
