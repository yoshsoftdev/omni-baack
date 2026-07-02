-- Phase 12.7: team inbox collaboration (activity timeline + internal notes)

DO $$
BEGIN
    CREATE TYPE conversation_activity_type AS ENUM (
        'assigned',
        'reassigned',
        'opened',
        'closed',
        'reopened',
        'note_added'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS conversation_activities (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    activity_type conversation_activity_type NOT NULL,
    actor_id UUID REFERENCES users (id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_activities_conversation
    ON conversation_activities (conversation_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_activities_team
    ON conversation_activities (team_id, created_at DESC);

CREATE TABLE IF NOT EXISTS conversation_notes (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_conversation_notes_conversation
    ON conversation_notes (conversation_id, created_at DESC)
    WHERE deleted_at IS NULL;

INSERT INTO permissions (id, code, name, description)
SELECT gen_random_uuid(), v.code, v.name, v.description
FROM (VALUES
    ('conversation.close', 'Close Conversation', 'Close or resolve inbox conversations'),
    ('conversation.note', 'Conversation Notes', 'Create and view internal conversation notes')
) AS v(code, name, description)
WHERE NOT EXISTS (SELECT 1 FROM permissions p WHERE LOWER(p.code) = LOWER(v.code));

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('owner', 'admin', 'agent')
  AND p.code IN ('conversation.close', 'conversation.note')
  AND r.deleted_at IS NULL
ON CONFLICT DO NOTHING;
