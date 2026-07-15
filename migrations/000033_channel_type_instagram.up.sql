ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS channel_type VARCHAR(32) NOT NULL DEFAULT 'whatsapp';

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS ig_user_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_channels_team_channel_type
    ON channels (team_id, channel_type)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_team_ig_user_id_active
    ON channels (team_id, ig_user_id)
    WHERE deleted_at IS NULL AND ig_user_id IS NOT NULL;
