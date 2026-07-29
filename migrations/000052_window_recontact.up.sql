-- Window recontact: auto free-text before Meta 24h customer-care window closes.
ALTER TABLE team_inbox_settings
    ADD COLUMN IF NOT EXISTS window_recontact_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS window_recontact_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS window_recontact_after_hours INTEGER NOT NULL DEFAULT 23;

DO $$
BEGIN
    ALTER TABLE team_inbox_settings
        ADD CONSTRAINT team_inbox_settings_window_recontact_after_hours_check
        CHECK (window_recontact_after_hours >= 1 AND window_recontact_after_hours <= 23);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS inbox_window_recontact_sends (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    customer_message_at TIMESTAMPTZ NOT NULL,
    message_id UUID,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT inbox_window_recontact_sends_unique
        UNIQUE (conversation_id, customer_message_at)
);

CREATE INDEX IF NOT EXISTS idx_inbox_window_recontact_sends_team_sent
    ON inbox_window_recontact_sends (team_id, sent_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversations_window_recontact_due
    ON conversations (team_id, last_customer_message_at)
    WHERE deleted_at IS NULL
      AND last_customer_message_at IS NOT NULL
      AND status IN ('open', 'pending');
