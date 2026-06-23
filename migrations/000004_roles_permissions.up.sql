-- Phase 9A: roles, permissions, RBAC seeds
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

CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);

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
    ('c0000000-0000-4000-8000-000000000011', 'contact.import', 'Import Contacts', 'Import contacts from files'),
    ('c0000000-0000-4000-8000-000000000012', 'conversation.view', 'View Conversations', 'View conversations'),
    ('c0000000-0000-4000-8000-000000000013', 'conversation.reply', 'Reply Conversations', 'Send messages'),
    ('c0000000-0000-4000-8000-000000000014', 'conversation.assign', 'Assign Conversations', 'Assign conversations'),
    ('c0000000-0000-4000-8000-000000000015', 'campaign.view', 'View Campaigns', 'View campaigns'),
    ('c0000000-0000-4000-8000-000000000016', 'campaign.create', 'Create Campaigns', 'Create campaigns'),
    ('c0000000-0000-4000-8000-000000000017', 'campaign.update', 'Update Campaigns', 'Update campaigns'),
    ('c0000000-0000-4000-8000-000000000018', 'campaign.schedule', 'Schedule Campaigns', 'Schedule campaigns'),
    ('c0000000-0000-4000-8000-000000000019', 'campaign.send', 'Send Campaigns', 'Send campaigns'),
    ('c0000000-0000-4000-8000-000000000020', 'campaign.cancel', 'Cancel Campaigns', 'Cancel campaigns'),
    ('c0000000-0000-4000-8000-000000000021', 'campaign.delete', 'Delete Campaigns', 'Delete draft campaigns'),
    ('c0000000-0000-4000-8000-000000000022', 'template.view', 'View Templates', 'View templates'),
    ('c0000000-0000-4000-8000-000000000023', 'template.create', 'Create Templates', 'Submit templates to Meta'),
    ('c0000000-0000-4000-8000-000000000024', 'template.sync', 'Sync Templates', 'Sync templates from Meta'),
    ('c0000000-0000-4000-8000-000000000025', 'channel.view', 'View Channels', 'View WhatsApp channels'),
    ('c0000000-0000-4000-8000-000000000026', 'channel.manage', 'Manage Channels', 'Manage WhatsApp channels'),
    ('c0000000-0000-4000-8000-000000000027', 'device.view', 'View Devices', 'Legacy alias for channel.view'),
    ('c0000000-0000-4000-8000-000000000028', 'device.manage', 'Manage Devices', 'Legacy alias for channel.manage'),
    ('c0000000-0000-4000-8000-000000000029', 'segment.view', 'View Segments', 'View segments'),
    ('c0000000-0000-4000-8000-000000000030', 'segment.create', 'Create Segments', 'Create segments'),
    ('c0000000-0000-4000-8000-000000000031', 'segment.update', 'Update Segments', 'Update segments'),
    ('c0000000-0000-4000-8000-000000000032', 'segment.delete', 'Delete Segments', 'Delete segments'),
    ('c0000000-0000-4000-8000-000000000033', 'tag.view', 'View Tags', 'View tags'),
    ('c0000000-0000-4000-8000-000000000034', 'tag.create', 'Create Tags', 'Create tags'),
    ('c0000000-0000-4000-8000-000000000035', 'tag.update', 'Update Tags', 'Update tags'),
    ('c0000000-0000-4000-8000-000000000036', 'tag.delete', 'Delete Tags', 'Delete tags'),
    ('c0000000-0000-4000-8000-000000000037', 'report.view', 'View Reports', 'View reports'),
    ('c0000000-0000-4000-8000-000000000038', 'inbox.view', 'View Inbox', 'View inbox'),
    ('c0000000-0000-4000-8000-000000000039', 'inbox.send', 'Send Inbox Messages', 'Send inbox messages'),
    ('c0000000-0000-4000-8000-000000000040', 'inbox.update', 'Update Inbox', 'Assign and update conversations'),
    ('c0000000-0000-4000-8000-000000000041', 'billing.view', 'View Billing', 'View billing'),
    ('c0000000-0000-4000-8000-000000000042', 'billing.manage', 'Manage Billing', 'Manage billing');

INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000001', id FROM permissions;

INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000002', id FROM permissions
WHERE code NOT IN ('workspace.manage', 'billing.view', 'billing.manage');

INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000003', id FROM permissions
WHERE code IN (
    'team.manage', 'user.view', 'user.update',
    'contact.view', 'contact.import',
    'conversation.view', 'conversation.reply', 'conversation.assign',
    'campaign.view', 'template.view', 'channel.view', 'device.view',
    'segment.view', 'tag.view', 'report.view', 'inbox.view', 'inbox.send', 'inbox.update'
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT 'b0000000-0000-4000-8000-000000000004', id FROM permissions
WHERE code IN (
    'contact.view', 'conversation.view', 'conversation.reply',
    'template.view', 'channel.view', 'device.view', 'inbox.view', 'inbox.send'
);
