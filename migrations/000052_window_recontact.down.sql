DROP INDEX IF EXISTS idx_conversations_window_recontact_due;
DROP INDEX IF EXISTS idx_inbox_window_recontact_sends_team_sent;
DROP TABLE IF EXISTS inbox_window_recontact_sends;

ALTER TABLE team_inbox_settings
    DROP CONSTRAINT IF EXISTS team_inbox_settings_window_recontact_after_hours_check;

ALTER TABLE team_inbox_settings
    DROP COLUMN IF EXISTS window_recontact_after_hours,
    DROP COLUMN IF EXISTS window_recontact_text,
    DROP COLUMN IF EXISTS window_recontact_enabled;
