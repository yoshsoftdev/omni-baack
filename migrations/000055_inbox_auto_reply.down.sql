DROP TABLE IF EXISTS inbox_auto_replies;
DROP TABLE IF EXISTS inbox_keyword_rules;
ALTER TABLE team_inbox_settings
    DROP COLUMN IF EXISTS welcome_text,
    DROP COLUMN IF EXISTS welcome_enabled;
