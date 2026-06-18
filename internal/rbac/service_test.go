package rbac

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type mockRepo struct {
	matrix map[string]map[string]bool
	roles  []Role
	perms  []Permission
}

func (m *mockRepo) LoadRolePermissions(ctx context.Context) (map[string]map[string]bool, error) {
	return m.matrix, nil
}

func (m *mockRepo) ListRoles(ctx context.Context) ([]Role, error) { return m.roles, nil }

func (m *mockRepo) FindRoleByID(ctx context.Context, id uuid.UUID) (*Role, error) {
	for _, r := range m.roles {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockRepo) FindRoleByName(ctx context.Context, name string) (*Role, error) {
	for _, r := range m.roles {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockRepo) ListPermissions(ctx context.Context) ([]Permission, error) { return m.perms, nil }

func (m *mockRepo) ListPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]Permission, error) {
	return nil, nil
}

func (m *mockRepo) GetPermissionMatrix(ctx context.Context) ([]PermissionMatrixRow, error) {
	return nil, nil
}

func TestHasPermission(t *testing.T) {
	svc := NewService(&mockRepo{
		matrix: map[string]map[string]bool{
			"owner": {PermCampaignCreate: true},
			"agent": {PermCampaignCreate: false, PermConversationReply: true},
		},
	})
	_ = svc.WarmCache(context.Background())

	ok, err := svc.HasPermission(context.Background(), "owner", PermCampaignCreate)
	if err != nil || !ok {
		t.Fatalf("owner should have campaign.create")
	}

	ok, err = svc.HasPermission(context.Background(), "agent", PermCampaignCreate)
	if err != nil || ok {
		t.Fatalf("agent should not have campaign.create")
	}

	ok, err = svc.HasPermission(context.Background(), "agent", PermConversationReply)
	if err != nil || !ok {
		t.Fatalf("agent should have conversation.reply")
	}
}
