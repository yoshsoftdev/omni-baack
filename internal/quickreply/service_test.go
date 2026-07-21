package quickreply

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type mockQuickReplyRepo struct {
	items map[uuid.UUID]*QuickReply
}

func (m *mockQuickReplyRepo) Create(ctx context.Context, item *QuickReply) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockQuickReplyRepo) FindByID(ctx context.Context, teamID, id uuid.UUID) (*QuickReply, error) {
	item, ok := m.items[id]
	if !ok || item.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	return item, nil
}

func (m *mockQuickReplyRepo) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[QuickReply], error) {
	return repository.Page[QuickReply]{}, nil
}

func (m *mockQuickReplyRepo) Update(ctx context.Context, item *QuickReply) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockQuickReplyRepo) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	return nil
}

type mockTeamRepo struct {
	members map[string]team.Member
}

func (m *mockTeamRepo) Create(ctx context.Context, t *team.Team) error { return nil }
func (m *mockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*team.Team, error) {
	return nil, team.ErrNotFound
}
func (m *mockTeamRepo) Update(ctx context.Context, t *team.Team) error { return nil }
func (m *mockTeamRepo) CreateMember(ctx context.Context, member *team.Member) error { return nil }
func (m *mockTeamRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *mockTeamRepo) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*team.Member, error) {
	member, ok := m.members[teamID.String()+":"+userID.String()]
	if !ok {
		return nil, team.ErrMemberNotFound
	}
	return &member, nil
}
func (m *mockTeamRepo) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *mockTeamRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *mockTeamRepo) ListMembersFiltered(ctx context.Context, teamID uuid.UUID, filter team.ListMembersFilter) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *mockTeamRepo) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role team.Role, roleID uuid.UUID) error {
	return nil
}
func (m *mockTeamRepo) UpdateMemberStatus(ctx context.Context, teamID, memberID uuid.UUID, status team.MemberStatus) error {
	return nil
}
func (m *mockTeamRepo) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error { return nil }
func (m *mockTeamRepo) FindRoleIDByName(ctx context.Context, name string) (uuid.UUID, error) {
	return uuid.Nil, team.ErrNotFound
}
func (m *mockTeamRepo) GetWorkspaceDetail(ctx context.Context, teamID uuid.UUID) (*team.WorkspaceDetail, error) {
	return nil, team.ErrNotFound
}
func (m *mockTeamRepo) CreateInvitation(ctx context.Context, inv *team.Invitation) error { return nil }
func (m *mockTeamRepo) ListInvitations(ctx context.Context, teamID uuid.UUID) ([]team.Invitation, error) {
	return nil, nil
}
func (m *mockTeamRepo) FindInvitationByID(ctx context.Context, teamID, invitationID uuid.UUID) (*team.Invitation, error) {
	return nil, team.ErrNotFound
}
func (m *mockTeamRepo) SoftDeleteInvitation(ctx context.Context, teamID, invitationID uuid.UUID) error {
	return nil
}
func (m *mockTeamRepo) UpdateInvitationRole(ctx context.Context, teamID, invitationID uuid.UUID, role team.Role, roleID uuid.UUID) error {
	return nil
}
func (m *mockTeamRepo) GetPendingInvitation(ctx context.Context, invitationID uuid.UUID) (*team.InvitationDetail, error) {
	return nil, team.ErrNotFound
}
func (m *mockTeamRepo) MarkInvitationAccepted(ctx context.Context, invitationID uuid.UUID) error {
	return nil
}

func newTestRBAC() *rbac.Service {
	svc := rbac.NewService(&rbacMockRepo{
		matrix: map[string]map[string]bool{
			"owner": {
				rbac.PermQuickReplyView:   true,
				rbac.PermQuickReplyCreate: true,
				rbac.PermQuickReplyUpdate: true,
				rbac.PermQuickReplyDelete: true,
			},
			"admin": {
				rbac.PermQuickReplyView:   true,
				rbac.PermQuickReplyCreate: true,
				rbac.PermQuickReplyUpdate: true,
				rbac.PermQuickReplyDelete: true,
			},
			"agent": {
				rbac.PermQuickReplyView: true,
			},
		},
	})
	_ = svc.WarmCache(context.Background())
	return svc
}

type rbacMockRepo struct {
	matrix map[string]map[string]bool
}

func (m *rbacMockRepo) LoadRolePermissions(ctx context.Context) (map[string]map[string]bool, error) {
	return m.matrix, nil
}
func (m *rbacMockRepo) ListRoles(ctx context.Context) ([]rbac.Role, error) { return nil, nil }
func (m *rbacMockRepo) FindRoleByID(ctx context.Context, id uuid.UUID) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (m *rbacMockRepo) FindRoleByName(ctx context.Context, name string) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (m *rbacMockRepo) ListPermissions(ctx context.Context) ([]rbac.Permission, error) { return nil, nil }
func (m *rbacMockRepo) ListPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]rbac.Permission, error) {
	return nil, nil
}
func (m *rbacMockRepo) GetPermissionMatrix(ctx context.Context) ([]rbac.PermissionMatrixRow, error) {
	return nil, nil
}
func (m *rbacMockRepo) CreateRole(ctx context.Context, role *rbac.Role) error { return nil }
func (m *rbacMockRepo) UpdateRole(ctx context.Context, role *rbac.Role) error { return nil }
func (m *rbacMockRepo) SoftDeleteRole(ctx context.Context, id uuid.UUID) error { return nil }
func (m *rbacMockRepo) ReplaceRolePermissions(ctx context.Context, roleID uuid.UUID, permissionCodes []string) error {
	return nil
}

func TestService_Create_AllowsOwner(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(
		&mockQuickReplyRepo{items: map[uuid.UUID]*QuickReply{}},
		&mockTeamRepo{members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {Role: team.RoleOwner},
		}},
		newTestRBAC(),
	)

	item, err := svc.Create(context.Background(), teamID, userID, string(team.RoleOwner), CreateInput{
		Category: "Greeting",
		Content:  "Hello {{customer_name}}",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.Category != "Greeting" {
		t.Fatalf("category = %q", item.Category)
	}
}

func TestService_Create_ForbidsAgent(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(
		&mockQuickReplyRepo{items: map[uuid.UUID]*QuickReply{}},
		&mockTeamRepo{members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {Role: team.RoleAgent},
		}},
		newTestRBAC(),
	)

	_, err := svc.Create(context.Background(), teamID, userID, string(team.RoleAgent), CreateInput{
		Content: "Hello",
	})
	if !errors.Is(err, serviceerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestService_List_AllowsAgent(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(
		&mockQuickReplyRepo{items: map[uuid.UUID]*QuickReply{}},
		&mockTeamRepo{members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {Role: team.RoleAgent},
		}},
		newTestRBAC(),
	)

	_, err := svc.List(context.Background(), teamID, userID, string(team.RoleAgent), ListFilter{}, repository.PageParams{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestService_Delete_ForbidsAgent(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(
		&mockQuickReplyRepo{items: map[uuid.UUID]*QuickReply{}},
		&mockTeamRepo{members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {Role: team.RoleAgent},
		}},
		newTestRBAC(),
	)

	err := svc.Delete(context.Background(), teamID, userID, string(team.RoleAgent), uuid.New())
	if !errors.Is(err, serviceerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
