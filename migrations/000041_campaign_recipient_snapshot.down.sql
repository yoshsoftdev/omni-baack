ALTER TABLE campaign_recipients
    DROP COLUMN IF EXISTS variables,
    DROP COLUMN IF EXISTS wa_id,
    DROP COLUMN IF EXISTS provider_profile_id;

ALTER TABLE campaign_recipients RENAME COLUMN phone_number TO phone;
