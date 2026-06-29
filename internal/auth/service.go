package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/password"
	"github.com/omnichannel/omni-chan-v2/pkg/slug"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvitationMismatch = errors.New("email does not match invitation")
)

type Service struct {
	users    user.Repository
	teams    team.Repository
	refresh  RefreshRepository
	jwt      *jwtmanager.Manager
	refreshTTL time.Duration
}

func NewService(
	users user.Repository,
	teams team.Repository,
	refresh RefreshRepository,
	jwt *jwtmanager.Manager,
	cfg config.JWTConfig,
) *Service {
	return &Service{
		users:      users,
		teams:      teams,
		refresh:    refresh,
		jwt:        jwt,
		refreshTTL: cfg.RefreshTTL,
	}
}

type RegisterInput struct {
	Name     string
	Email    string
	Password string
	TeamName string
}

type LoginInput struct {
	Email    string
	Password string
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*Session, error) {
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}

	hash, err := password.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	userID := uuid.New()
	teamID := uuid.New()
	teamName := strings.TrimSpace(input.TeamName)
	if teamName == "" {
		teamName = fmt.Sprintf("%s's Team", strings.TrimSpace(input.Name))
	}

	newUser := &user.User{
		ID:           userID,
		Email:        strings.ToLower(strings.TrimSpace(input.Email)),
		PasswordHash: hash,
		Name:         strings.TrimSpace(input.Name),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	newTeam := &team.Team{
		ID:           teamID,
		Name:         teamName,
		Slug:         slug.FromName(teamName),
		OwnerID:      &userID,
		Timezone:     "Asia/Jakarta",
		Country:      "ID",
		PlanType:     team.PlanStarter,
		ContactLimit: 50000,
		ChannelLimit: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.users.Create(ctx, newUser); err != nil {
		return nil, err
	}

	if err := s.teams.Create(ctx, newTeam); err != nil {
		newTeam.Slug = fmt.Sprintf("%s-%s", newTeam.Slug, userID.String()[:8])
		if err := s.teams.Create(ctx, newTeam); err != nil {
			return nil, err
		}
	}

	roleID, err := s.teams.FindRoleIDByName(ctx, string(team.RoleOwner))
	if err != nil {
		return nil, err
	}

	member := &team.Member{
		ID:        uuid.New(),
		TeamID:    teamID,
		UserID:    userID,
		Role:      team.RoleOwner,
		RoleID:    roleID,
		Status:    team.MemberStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.teams.CreateMember(ctx, member); err != nil {
		return nil, err
	}

	return s.issueSession(ctx, newUser, newTeam, string(team.RoleOwner))
}

type AcceptInviteInput struct {
	InvitationID uuid.UUID
	Name         string
	Password     string
}

func (s *Service) GetInvitation(ctx context.Context, invitationID uuid.UUID) (*team.InvitationDetail, error) {
	detail, err := s.teams.GetPendingInvitation(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(detail.CreatedAt.Add(7 * 24 * time.Hour)) {
		return nil, team.ErrInvitationExpired
	}
	return detail, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, input AcceptInviteInput) (*Session, error) {
	detail, err := s.GetInvitation(ctx, input.InvitationID)
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}

	invitedTeam, err := s.teams.FindByID(ctx, detail.TeamID)
	if err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(detail.Email))
	found, err := s.users.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	if errors.Is(err, user.ErrNotFound) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = strings.TrimSpace(detail.Name)
		}
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}

		hash, err := password.Hash(input.Password)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}

		newUser := &user.User{
			ID:           uuid.New(),
			Email:        email,
			PasswordHash: hash,
			Name:         name,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.users.Create(ctx, newUser); err != nil {
			return nil, err
		}
		found = newUser
	} else {
		if !password.Verify(found.PasswordHash, input.Password) {
			return nil, ErrInvalidCredentials
		}
		if strings.ToLower(found.Email) != email {
			return nil, ErrInvitationMismatch
		}
	}

	if _, err := s.teams.FindMemberByUser(ctx, detail.TeamID, found.ID); err == nil {
		return nil, team.ErrAlreadyMember
	} else if !errors.Is(err, team.ErrMemberNotFound) {
		return nil, err
	}

	member := &team.Member{
		ID:        uuid.New(),
		TeamID:    detail.TeamID,
		UserID:    found.ID,
		Role:      detail.Role,
		RoleID:    detail.RoleID,
		Status:    team.MemberStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.teams.CreateMember(ctx, member); err != nil {
		return nil, err
	}
	if err := s.teams.MarkInvitationAccepted(ctx, input.InvitationID); err != nil {
		return nil, err
	}

	return s.issueSession(ctx, found, invitedTeam, string(detail.Role))
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*Session, error) {
	found, err := s.users.FindByEmail(ctx, strings.TrimSpace(input.Email))
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !password.Verify(found.PasswordHash, input.Password) {
		return nil, ErrInvalidCredentials
	}

	member, err := s.teams.FindPrimaryMembership(ctx, found.ID)
	if err != nil {
		return nil, err
	}

	t, err := s.teams.FindByID(ctx, member.TeamID)
	if err != nil {
		return nil, err
	}

	return s.issueSession(ctx, found, t, string(member.Role))
}

func (s *Service) Refresh(ctx context.Context, rawToken string) (*Session, error) {
	hash := hashToken(rawToken)
	stored, err := s.refresh.FindByHash(ctx, hash)
	if err != nil {
		return nil, ErrInvalidRefresh
	}
	if stored.RevokedAt != nil || time.Now().After(stored.ExpiresAt) {
		return nil, ErrInvalidRefresh
	}

	_ = s.refresh.Revoke(ctx, hash)

	found, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, err
	}

	t, err := s.teams.FindByID(ctx, stored.TeamID)
	if err != nil {
		return nil, err
	}

	member, err := s.teams.FindPrimaryMembership(ctx, found.ID)
	if err != nil {
		return nil, err
	}

	role := string(member.Role)
	if member.TeamID != t.ID {
		role = string(team.RoleAgent)
	}

	return s.issueSession(ctx, found, t, role)
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.refresh.Revoke(ctx, hashToken(rawToken))
}

func (s *Service) Me(ctx context.Context, userID, teamID uuid.UUID, role string) (*MeResponse, error) {
	found, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		return nil, err
	}

	return &MeResponse{
		User: UserProfile{
			ID:    found.ID,
			Email: found.Email,
			Name:  found.Name,
		},
		Team: TeamProfile{
			ID:   t.ID,
			Name: t.Name,
			Slug: t.Slug,
		},
		Role: role,
	}, nil
}

func (s *Service) issueSession(ctx context.Context, u *user.User, t *team.Team, role string) (*Session, error) {
	access, _, err := s.jwt.GenerateAccess(u.ID, t.ID, u.Email, role)
	if err != nil {
		return nil, err
	}

	refreshRaw, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	refresh := &RefreshToken{
		ID:        uuid.New(),
		UserID:    u.ID,
		TeamID:    t.ID,
		TokenHash: hashToken(refreshRaw),
		ExpiresAt: now.Add(s.refreshTTL),
		CreatedAt: now,
	}
	if err := s.refresh.Create(ctx, refresh); err != nil {
		return nil, err
	}

	return &Session{
		UserID:       u.ID,
		TeamID:       t.ID,
		Email:        u.Email,
		Name:         u.Name,
		TeamName:     t.Name,
		TeamSlug:     t.Slug,
		Role:         role,
		AccessToken:  access,
		RefreshToken: refreshRaw,
	}, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	return nil
}

func generateRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
