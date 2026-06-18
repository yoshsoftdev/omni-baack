DROP TABLE IF EXISTS team_invitations;
ALTER TABLE team_members DROP COLUMN IF EXISTS status;
ALTER TABLE team_members DROP COLUMN IF EXISTS role_id;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
