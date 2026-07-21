CREATE TABLE quick_replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    category VARCHAR(100) NOT NULL DEFAULT 'General',
    content TEXT NOT NULL,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_quick_replies_team ON quick_replies (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_quick_replies_created_at ON quick_replies (created_at DESC) WHERE deleted_at IS NULL;

INSERT INTO permissions (id, code, name, description)
SELECT gen_random_uuid(), v.code, v.name, v.description
FROM (VALUES
    ('quickreply.view', 'View Quick Replies', 'View team quick reply templates'),
    ('quickreply.create', 'Create Quick Replies', 'Create team quick reply templates'),
    ('quickreply.update', 'Update Quick Replies', 'Update team quick reply templates'),
    ('quickreply.delete', 'Delete Quick Replies', 'Delete team quick reply templates')
) AS v(code, name, description)
WHERE NOT EXISTS (SELECT 1 FROM permissions p WHERE LOWER(p.code) = LOWER(v.code));

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('owner', 'admin', 'agent', 'supervisor')
  AND p.code = 'quickreply.view'
  AND r.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('owner', 'admin')
  AND p.code IN ('quickreply.create', 'quickreply.update', 'quickreply.delete')
  AND r.deleted_at IS NULL
ON CONFLICT DO NOTHING;
