-- Campaign recipient identity/variable snapshot (Canonical Provider migration).
-- provider_profile_id is audit/tracing only — no FK to provider_profiles.
-- Snapshot fields are immutable after insert; delivery columns remain mutable.

ALTER TABLE campaign_recipients RENAME COLUMN phone TO phone_number;

ALTER TABLE campaign_recipients
    ADD COLUMN IF NOT EXISTS provider_profile_id UUID,
    ADD COLUMN IF NOT EXISTS wa_id VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS variables JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE campaign_recipients
SET wa_id = phone_number
WHERE wa_id = '' AND phone_number <> '';

COMMENT ON COLUMN campaign_recipients.provider_profile_id IS
    'Audit/tracing only. Not used at send runtime. No FK by design.';
COMMENT ON COLUMN campaign_recipients.phone_number IS
    'Frozen WhatsApp recipient address (Meta to). Immutable after insert.';
COMMENT ON COLUMN campaign_recipients.wa_id IS
    'Frozen WhatsApp provider_user_id. Immutable after insert.';
COMMENT ON COLUMN campaign_recipients.variables IS
    'Flat resolved template variable key-value snapshot. Immutable after insert.';
