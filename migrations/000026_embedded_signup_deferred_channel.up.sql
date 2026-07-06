-- Deferred channel creation: signup sessions may exist before a channel row is provisioned.
ALTER TABLE embedded_signup_sessions
    ALTER COLUMN channel_id DROP NOT NULL;

ALTER TABLE embedded_signup_sessions
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_embedded_signup_team_user_created
    ON embedded_signup_sessions (team_id, user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

DROP INDEX IF EXISTS idx_embedded_signup_oauth_code_hash;
CREATE UNIQUE INDEX idx_embedded_signup_oauth_code_hash_team
    ON embedded_signup_sessions (team_id, oauth_code_hash)
    WHERE oauth_code_hash IS NOT NULL;
