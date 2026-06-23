ALTER TABLE devices
    ADD COLUMN connection_type VARCHAR(32) NOT NULL DEFAULT 'manual',
    ADD COLUMN is_coex BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN onboarding_status VARCHAR(32),
    ADD COLUMN contacts_synced_count INT NOT NULL DEFAULT 0,
    ADD COLUMN history_messages_synced INT NOT NULL DEFAULT 0,
    ADD COLUMN history_sync_progress INT NOT NULL DEFAULT 0,
    ADD COLUMN contacts_sync_request_id VARCHAR(255),
    ADD COLUMN history_sync_request_id VARCHAR(255),
    ADD COLUMN onboarding_error TEXT,
    ADD COLUMN onboarding_completed_at TIMESTAMPTZ;

CREATE INDEX idx_devices_onboarding_status ON devices (onboarding_status)
    WHERE deleted_at IS NULL AND onboarding_status IS NOT NULL;
