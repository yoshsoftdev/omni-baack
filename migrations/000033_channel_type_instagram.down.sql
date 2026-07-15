DROP INDEX IF EXISTS idx_channels_team_ig_user_id_active;
DROP INDEX IF EXISTS idx_channels_team_channel_type;

ALTER TABLE channels
    DROP COLUMN IF EXISTS ig_user_id;

ALTER TABLE channels
    DROP COLUMN IF EXISTS channel_type;
