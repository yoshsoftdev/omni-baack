DROP TABLE IF EXISTS conversation_notes;
DROP TABLE IF EXISTS conversation_activities;
DROP TYPE IF EXISTS conversation_activity_type;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN ('conversation.close', 'conversation.note')
);

DELETE FROM permissions WHERE code IN ('conversation.close', 'conversation.note');
