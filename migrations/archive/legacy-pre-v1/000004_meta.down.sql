DROP TABLE IF EXISTS inbox_messages;

DROP INDEX IF EXISTS idx_devices_phone_number_id;

ALTER TABLE devices
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS access_token,
    DROP COLUMN IF EXISTS phone_number_id,
    DROP COLUMN IF EXISTS waba_id;
