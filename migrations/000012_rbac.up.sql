ALTER TYPE team_role ADD VALUE IF NOT EXISTS 'supervisor';

CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_roles_name_active ON roles (LOWER(name)) WHERE deleted_at IS NULL;

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(100) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_permissions_code ON permissions (LOWER(code));

CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

ALTER TABLE team_members
    ADD COLUMN IF NOT EXISTS role_id UUID REFERENCES roles (id),
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active';

CREATE TABLE team_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role_id UUID NOT NULL REFERENCES roles (id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    invited_by UUID NOT NULL REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_team_invitations_team_id ON team_invitations (team_id) WHERE deleted_at IS NULL;

INSERT INTO roles (id, name, description, is_system) VALUES
    ('b0000000-0000-4000-8000-000000000001', 'owner', 'Full workspace access', TRUE),
    ('b0000000-0000-4000-8000-000000000002', 'admin', 'Manage users and operations', TRUE),
    ('b0000000-0000-4000-8000-000000000003', 'supervisor', 'Supervise agents and conversations', TRUE),
    ('b0000000-0000-4000-8000-000000000004', 'agent', 'Handle assigned conversations', TRUE);

INSERT INTO permissions (id, code, name, description) VALUES
    ('c0000000-0000-4000-8000-000000000001', 'workspace.manage', 'Manage Workspace', 'Update workspace settings'),
    ('c0000000-0000-4000-8000-000000000002', 'team.manage', 'Manage Team', 'Manage team structure'),
    ('c0000000-0000-4000-8000-000000000003', 'user.view', 'View Users', 'View team members'),
    ('c0000000-0000-4000-8000-000000000004', 'user.invite', 'Invite Users', 'Invite new team members'),
    ('c0000000-0000-4000-8000-000000000005', 'user.update', 'Update Users', 'Update member roles and status'),
    ('c0000000-0000-4000-8000-000000000006', 'user.delete', 'Remove Users', 'Remove team members'),
    ('c0000000-0000-4000-8000-000000000007', 'contact.view', 'View Contacts', 'View contacts'),
    ('c0000000-0000-4000-8000-000000000008', 'contact.create', 'Create Contacts', 'Create contacts'),
    ('c0000000-0000-4000-8000-000000000009', 'contact.update', 'Update Contacts', 'Update contacts'),
    ('c0000000-0000-4000-8000-000000000010', 'contact.delete', 'Delete Contacts', 'Delete contacts'),
    ('c0000000-0000-4000-8000-000000000011', 'conversation.view', 'View Conversations', 'View conversations'),
    ('c0000000-0000-4000-8000-000000000012', 'conversation.reply', 'Reply Conversations', 'Send messages'),
    ('c0000000-0000-4000-8000-000000000013', 'conversation.assign', 'Assign Conversations', 'Assign conversations'),
    ('c0000000-0000-4000-8000-000000000014', 'campaign.view', 'View Campaigns', 'View campaigns'),
    ('c0000000-0000-4000-8000-000000000015', 'campaign.create', 'Create Campaigns', 'Create campaigns'),
    ('c0000000-0000-4000-8000-000000000016', 'campaign.send', 'Send Campaigns', 'Send campaigns'),
    ('c0000000-0000-4000-8000-000000000017', 'template.view', 'View Templates', 'View templates'),
    ('c0000000-0000-4000-8000-000000000018', 'template.create', 'Create Templates', 'Create templates'),
    ('c0000000-0000-4000-8000-000000000019', 'template.sync', 'Sync Templates', 'Sync templates from Meta'),
    ('c0000000-0000-4000-8000-000000000020', 'report.view', 'View Reports', 'View reports'),
    ('c0000000-0000-4000-8000-000000000021', 'billing.view', 'View Billing', 'View billing'),
    ('c0000000-0000-4000-8000-000000000022', 'billing.manage', 'Manage Billing', 'Manage billing'),
    ('c0000000-0000-4000-8000-000000000023', 'device.view', 'View Devices', 'View WhatsApp devices'),
    ('c0000000-0000-4000-8000-000000000024', 'device.manage', 'Manage Devices', 'Manage WhatsApp devices');

-- owner: all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000001', id FROM permissions;

-- admin: all except workspace.manage and billing.*
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000002', id FROM permissions
WHERE code NOT IN ('workspace.manage', 'billing.view', 'billing.manage');

-- supervisor
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000003', id FROM permissions
WHERE code IN (
    'team.manage', 'user.view', 'user.update',
    'contact.view', 'conversation.view', 'conversation.reply', 'conversation.assign',
    'campaign.view', 'template.view', 'report.view', 'device.view'
);

-- agent
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000004', id FROM permissions
WHERE code IN (
    'contact.view', 'conversation.view', 'conversation.reply',
    'template.view', 'device.view'
);

UPDATE team_members tm
SET role_id = r.id
FROM roles r
WHERE tm.role_id IS NULL AND r.name = tm.role::text;

UPDATE team_members
SET role_id = 'b0000000-0000-4000-8000-000000000004'
WHERE role_id IS NULL;

ALTER TABLE team_members ALTER COLUMN role_id SET NOT NULL;
