package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/password"
)

type mockUserRepo struct {
	users map[string]*user.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *user.User) error {
	if _, ok := m.users[u.Email]; ok {
		return user.ErrEmailTaken
	}
	m.users[u.Email] = u
	return nil
}

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

func (m *mockUserRepo) Update(ctx context.Context, u *user.User) error { return nil }

func (m *mockUserRepo) UpdateLastActive(ctx context.Context, id uuid.UUID, at time.Time) error {
	return nil
}

func (m *mockUserRepo) SoftDelete(ctx context.Context, id uuid.UUID) error { return nil }

func (m *mockUserRepo) CreateRefreshToken(ctx context.Context, t *user.RefreshToken) error { return nil }

func (m *mockUserRepo) FindRefreshTokenByHash(ctx context.Context, hash string) (*user.RefreshToken, error) {
	return nil, user.ErrNotFound
}

func (m *mockUserRepo) RevokeRefreshToken(ctx context.Context, id uuid.UUID, at time.Time) error {
	return nil
}

type mockTeamRepo struct {
	teams   map[uuid.UUID]*team.Team
	members []team.Member
}

func (m *mockTeamRepo) Create(ctx context.Context, t *team.Team) error {
	m.teams[t.ID] = t
	return nil
}

func (m *mockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*team.Team, error) {
	t, ok := m.teams[id]
	if !ok {
		return nil, team.ErrNotFound
	}
	return t, nil
}

func (m *mockTeamRepo) CreateMember(ctx context.Context, member *team.Member) error {
	m.members = append(m.members, *member)
	return nil
}

func (m *mockTeamRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*team.Member, error) {
	for i := range m.members {
		if m.members[i].UserID == userID {
			return &m.members[i], nil
		}
	}
	return nil, team.ErrMemberNotFound
}

func (m *mockTeamRepo) Update(ctx context.Context, t *team.Team) error {
	m.teams[t.ID] = t
	return nil
}

func (m *mockTeamRepo) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*team.Member, error) {
	for i := range m.members {
		if m.members[i].TeamID == teamID && m.members[i].UserID == userID {
			return &m.members[i], nil
		}
	}
	return nil, team.ErrMemberNotFound
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

func (m *mockTeamRepo) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error {
	return nil
}

func (m *mockTeamRepo) FindRoleIDByName(ctx context.Context, name string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockTeamRepo) GetWorkspaceDetail(ctx context.Context, teamID uuid.UUID) (*team.WorkspaceDetail, error) {
	return nil, team.ErrNotFound
}

func (m *mockTeamRepo) CreateInvitation(ctx context.Context, inv *team.Invitation) error { return nil }

func (m *mockTeamRepo) ListInvitations(ctx context.Context, teamID uuid.UUID) ([]team.Invitation, error) {
	return nil, nil
}

type mockRefreshRepo struct {
	tokens map[string]*RefreshToken
}

func (m *mockRefreshRepo) Create(ctx context.Context, token *RefreshToken) error {
	m.tokens[token.TokenHash] = token
	return nil
}

func (m *mockRefreshRepo) FindByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	t, ok := m.tokens[hash]
	if !ok {
		return nil, ErrRefreshNotFound
	}
	return t, nil
}

func (m *mockRefreshRepo) Revoke(ctx context.Context, hash string) error {
	if t, ok := m.tokens[hash]; ok {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}

func TestService_RegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	userRepo := &mockUserRepo{users: map[string]*user.User{}}
	teamRepo := &mockTeamRepo{teams: map[uuid.UUID]*team.Team{}}
	refreshRepo := &mockRefreshRepo{tokens: map[string]*RefreshToken{}}
	jwt := jwtmanager.NewManager("access", "refresh", time.Minute)
	svc := NewService(userRepo, teamRepo, refreshRepo, jwt, config.JWTConfig{RefreshTTL: time.Hour})

	session, err := svc.Register(ctx, RegisterInput{
		Name:     "Jane",
		Email:    "jane@example.com",
		Password: "password123",
		TeamName: "Acme",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("expected tokens")
	}

	loginSession, err := svc.Login(ctx, LoginInput{
		Email:    "jane@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginSession.Email != "jane@example.com" {
		t.Fatalf("unexpected session: %+v", loginSession)
	}
}

func TestService_LoginInvalidPassword(t *testing.T) {
	ctx := context.Background()
	hash, _ := password.Hash("password123")
	userRepo := &mockUserRepo{
		users: map[string]*user.User{
			"bob@example.com": {
				ID:           uuid.New(),
				Email:        "bob@example.com",
				PasswordHash: hash,
				Name:         "Bob",
			},
		},
	}
	teamRepo := &mockTeamRepo{teams: map[uuid.UUID]*team.Team{}}
	refreshRepo := &mockRefreshRepo{tokens: map[string]*RefreshToken{}}
	jwt := jwtmanager.NewManager("access", "refresh", time.Minute)
	svc := NewService(userRepo, teamRepo, refreshRepo, jwt, config.JWTConfig{RefreshTTL: time.Hour})

	_, err := svc.Login(ctx, LoginInput{Email: "bob@example.com", Password: "wrong"})
	if err != ErrInvalidCredentials {
		t.Fatalf("Login() error = %v", err)
	}
}
