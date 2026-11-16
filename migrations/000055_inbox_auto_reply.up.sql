-- 9.2: welcome message and keyword auto-replies.
ALTER TABLE team_inbox_settings
    ADD COLUMN IF NOT EXISTS welcome_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS welcome_text TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS inbox_keyword_rules (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    keywords TEXT[] NOT NULL,
    match_type TEXT NOT NULL CHECK (match_type IN ('exact', 'contains')),
    reply_text TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inbox_keyword_rules_team_order
    ON inbox_keyword_rules (team_id, sort_order, created_at);

-- One automatic reply of each kind per inbound message: redelivered events never send twice.
CREATE TABLE IF NOT EXISTS inbox_auto_replies (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    inbound_message_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('welcome', 'keyword')),
    rule_id UUID REFERENCES inbox_keyword_rules (id) ON DELETE SET NULL,
    message_id UUID,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT inbox_auto_replies_unique UNIQUE (inbound_message_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_inbox_auto_replies_team_sent
    ON inbox_auto_replies (team_id, sent_at DESC);
