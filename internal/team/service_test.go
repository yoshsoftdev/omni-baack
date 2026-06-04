package team

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/user"
)

type mockTeamRepo struct {
	teams   map[uuid.UUID]*Team
	members []Member
}

func (m *mockTeamRepo) Create(ctx context.Context, team *Team) error {
	m.teams[team.ID] = team
	return nil
}

func (m *mockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	t, ok := m.teams[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (m *mockTeamRepo) Update(ctx context.Context, team *Team) error {
	m.teams[team.ID] = team
	return nil
}

func (m *mockTeamRepo) CreateMember(ctx context.Context, member *Member) error {
	for _, existing := range m.members {
		if existing.TeamID == member.TeamID && existing.UserID == member.UserID && existing.DeletedAt == nil {
			return ErrAlreadyMember
		}
	}
	m.members = append(m.members, *member)
	return nil
}

func (m *mockTeamRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*Member, error) {
	for i := range m.members {
		if m.members[i].UserID == userID {
			return &m.members[i], nil
		}
	}
	return nil, ErrMemberNotFound
}

func (m *mockTeamRepo) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*Member, error) {
	for i := range m.members {
		if m.members[i].TeamID == teamID && m.members[i].UserID == userID && m.members[i].DeletedAt == nil {
			return &m.members[i], nil
		}
	}
	return nil, ErrMemberNotFound
}

func (m *mockTeamRepo) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*Member, error) {
	for i := range m.members {
		if m.members[i].TeamID == teamID && m.members[i].ID == memberID {
			return &m.members[i], nil
		}
	}
	return nil, ErrMemberNotFound
}

func (m *mockTeamRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]MemberWithUser, error) {
	return nil, nil
}

func (m *mockTeamRepo) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role Role) error {
	return nil
}

func (m *mockTeamRepo) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error {
	return nil
}

type mockUserRepo struct {
	users map[string]*user.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *user.User) error { return nil }

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, user.ErrNotFound
}

func TestService_AddMember_ForbiddenForAgent(t *testing.T) {
	teamID := uuid.New()
	ownerID := uuid.New()
	repo := &mockTeamRepo{
		teams: map[uuid.UUID]*Team{teamID: {ID: teamID, Name: "Acme", Slug: "acme"}},
		members: []Member{
			{ID: uuid.New(), TeamID: teamID, UserID: ownerID, Role: RoleOwner},
		},
	}
	svc := NewService(repo, &mockUserRepo{users: map[string]*user.User{}})

	_, err := svc.AddMember(context.Background(), teamID, ownerID, RoleAgent, "new@example.com", RoleAdmin)
	if err != ErrForbidden {
		t.Fatalf("AddMember() error = %v", err)
	}
}

func TestService_Update_ForbiddenForAgent(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	repo := &mockTeamRepo{
		teams: map[uuid.UUID]*Team{
			teamID: {ID: teamID, Name: "Acme", Slug: "acme", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		members: []Member{
			{ID: uuid.New(), TeamID: teamID, UserID: userID, Role: RoleAgent},
		},
	}
	svc := NewService(repo, &mockUserRepo{})

	_, err := svc.Update(context.Background(), teamID, userID, RoleAgent, "New Name")
	if err != ErrForbidden {
		t.Fatalf("Update() error = %v", err)
	}
}
