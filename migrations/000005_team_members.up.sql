-- Phase 9A: team membership and invitations
CREATE TABLE team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role team_role NOT NULL DEFAULT 'agent',
    role_id UUID NOT NULL REFERENCES roles (id),
    status member_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_team_members_team_user_active ON team_members (team_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_team_members_team_id ON team_members (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_team_members_user_id ON team_members (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_team_members_role_id ON team_members (role_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_team_members_status ON team_members (team_id, status) WHERE deleted_at IS NULL;

CREATE TABLE team_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role_id UUID NOT NULL REFERENCES roles (id),
    status invitation_status NOT NULL DEFAULT 'pending',
    invited_by UUID NOT NULL REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_team_invitations_team_id ON team_invitations (team_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_team_invitations_email ON team_invitations (LOWER(email)) WHERE deleted_at IS NULL AND status = 'pending';
