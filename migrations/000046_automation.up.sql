CREATE TABLE automation_endpoints (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    slug VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    method VARCHAR(10) NOT NULL DEFAULT 'POST',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    action_type VARCHAR(64) NOT NULL,
    action_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CONSTRAINT automation_endpoints_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT automation_endpoints_method_check CHECK (method = 'POST')
);

CREATE UNIQUE INDEX idx_automation_endpoints_team_slug
    ON automation_endpoints (team_id, slug)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_automation_endpoints_team
    ON automation_endpoints (team_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_automation_endpoints_updated
    ON automation_endpoints (updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE automation_api_keys (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    key_prefix VARCHAR(32) NOT NULL,
    key_hash VARCHAR(128) NOT NULL,
    scopes JSONB NOT NULL DEFAULT '["send_template"]'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CONSTRAINT automation_api_keys_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX idx_automation_api_keys_team
    ON automation_api_keys (team_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_automation_api_keys_hash
    ON automation_api_keys (key_hash)
    WHERE deleted_at IS NULL;

CREATE TABLE automation_executions (
    id TEXT PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    endpoint_id TEXT NOT NULL REFERENCES automation_endpoints (id) ON DELETE CASCADE,
    api_key_id TEXT REFERENCES automation_api_keys (id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL,
    status_code INT NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    error_message TEXT,
    client_ip VARCHAR(64),
    provider_message_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT automation_executions_status_check CHECK (status IN ('queued', 'success', 'failed'))
);

CREATE INDEX idx_automation_executions_endpoint_created
    ON automation_executions (endpoint_id, created_at DESC);

CREATE INDEX idx_automation_executions_team_created
    ON automation_executions (team_id, created_at DESC);

INSERT INTO permissions (id, code, name, description)
SELECT gen_random_uuid(), v.code, v.name, v.description
FROM (VALUES
    ('automation.view', 'View Automation', 'View automation endpoints, keys, and analytics'),
    ('automation.create', 'Create Automation', 'Create automation endpoints and API keys'),
    ('automation.update', 'Update Automation', 'Update automation endpoints and API keys'),
    ('automation.delete', 'Delete Automation', 'Delete automation endpoints and API keys'),
    ('automation.invoke', 'Invoke Automation', 'Test-invoke automation endpoints from the dashboard')
) AS v(code, name, description)
WHERE NOT EXISTS (SELECT 1 FROM permissions p WHERE LOWER(p.code) = LOWER(v.code));

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('owner', 'admin', 'agent', 'supervisor')
  AND p.code = 'automation.view'
  AND r.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('owner', 'admin')
  AND p.code IN ('automation.create', 'automation.update', 'automation.delete', 'automation.invoke')
  AND r.deleted_at IS NULL
ON CONFLICT DO NOTHING;
