DROP INDEX IF EXISTS idx_devices_onboarding_status;

ALTER TABLE devices
    DROP COLUMN IF EXISTS connection_type,
    DROP COLUMN IF EXISTS is_coex,
    DROP COLUMN IF EXISTS onboarding_status,
    DROP COLUMN IF EXISTS contacts_synced_count,
    DROP COLUMN IF EXISTS history_messages_synced,
    DROP COLUMN IF EXISTS history_sync_progress,
    DROP COLUMN IF EXISTS contacts_sync_request_id,
    DROP COLUMN IF EXISTS history_sync_request_id,
    DROP COLUMN IF EXISTS onboarding_error,
    DROP COLUMN IF EXISTS onboarding_completed_at;
