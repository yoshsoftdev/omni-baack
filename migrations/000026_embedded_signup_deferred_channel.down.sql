DROP INDEX IF EXISTS idx_embedded_signup_oauth_code_hash_team;
CREATE UNIQUE INDEX idx_embedded_signup_oauth_code_hash
    ON embedded_signup_sessions (oauth_code_hash)
    WHERE oauth_code_hash IS NOT NULL;

DROP INDEX IF EXISTS idx_embedded_signup_team_user_created;

ALTER TABLE embedded_signup_sessions
    DROP COLUMN IF EXISTS user_id;

DELETE FROM embedded_signup_sessions WHERE channel_id IS NULL;

ALTER TABLE embedded_signup_sessions
    ALTER COLUMN channel_id SET NOT NULL;
