DROP TABLE IF EXISTS team_member_presence;
DROP TABLE IF EXISTS team_inbox_settings;
DROP TYPE IF EXISTS assignment_mode;

ALTER TABLE conversations
    DROP COLUMN IF EXISTS resolution_at,
    DROP COLUMN IF EXISTS last_agent_message_at,
    DROP COLUMN IF EXISTS last_customer_message_at,
    DROP COLUMN IF EXISTS first_response_at;

DROP INDEX IF EXISTS idx_conversations_team_unread;
DROP INDEX IF EXISTS idx_conversations_team_unassigned;
DROP INDEX IF EXISTS idx_conversations_team_status;
DROP INDEX IF EXISTS idx_conversations_sla_first_response;
