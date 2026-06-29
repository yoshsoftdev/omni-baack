package tag

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type mockTagRepo struct {
	tags map[uuid.UUID]*Tag
}

func (m *mockTagRepo) Create(ctx context.Context, tag *Tag) error {
	m.tags[tag.ID] = tag
	return nil
}
func (m *mockTagRepo) FindByID(ctx context.Context, teamID, tagID uuid.UUID) (*Tag, error) {
	t, ok := m.tags[tagID]
	if !ok || t.TeamID != teamID {
		return nil, ErrNotFound
	}
	return t, nil
}
func (m *mockTagRepo) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Tag, error) {
	return nil, nil
}
func (m *mockTagRepo) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Tag], error) {
	return repository.Page[Tag]{}, nil
}
func (m *mockTagRepo) AssignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error {
	return nil
}
func (m *mockTagRepo) UnassignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error {
	return nil
}
func (m *mockTagRepo) Update(ctx context.Context, tag *Tag) error { return nil }
func (m *mockTagRepo) SoftDelete(ctx context.Context, teamID, tagID uuid.UUID) error {
	return nil
}
func (m *mockTagRepo) ListContactIDsByTag(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
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

func TestService_Create(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(&mockTagRepo{tags: map[uuid.UUID]*Tag{}}, &mockTeamRepo{
		members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {Role: team.RoleOwner},
		},
	})

	tag, err := svc.Create(context.Background(), teamID, userID, string(team.RoleOwner), CreateInput{Name: "VIP"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if tag.Color != "#6366f1" {
		t.Fatalf("unexpected color %s", tag.Color)
	}
}
