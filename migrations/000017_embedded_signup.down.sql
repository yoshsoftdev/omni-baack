DROP TABLE IF EXISTS embedded_signup_sessions;
DROP TYPE IF EXISTS embedded_signup_status;

DROP INDEX IF EXISTS idx_channels_business_id;
ALTER TABLE channels DROP COLUMN IF EXISTS business_id;
