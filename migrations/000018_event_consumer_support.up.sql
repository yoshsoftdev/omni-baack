-- Phase 11.4: inbound message raw payload, contact source, channel health metadata
ALTER TYPE contact_source ADD VALUE IF NOT EXISTS 'meta_inbound';

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS raw_payload JSONB;

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS health_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_channels_phone_number_id_lookup
    ON channels (phone_number_id)
    WHERE deleted_at IS NULL AND phone_number_id IS NOT NULL;
