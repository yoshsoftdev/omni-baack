package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/user"
)

var (
	ErrForbidden         = errors.New("insufficient permissions")
	ErrInvalidRole       = errors.New("invalid role")
	ErrCannotModifyOwner = errors.New("cannot modify team owner")
	ErrUserNotFound      = errors.New("user not found, they must register first")
)

type Service struct {
	teams Repository
	users user.Repository
}

func NewService(teams Repository, users user.Repository) *Service {
	return &Service{teams: teams, users: users}
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID) (*Team, error) {
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.FindByID(ctx, teamID)
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole Role, name string) (*Team, error) {
	if !canManageTeam(actorRole) {
		return nil, ErrForbidden
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}

	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}

	team, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		return nil, err
	}

	team.Name = name
	team.Slug = slugify(name)
	team.UpdatedAt = time.Now().UTC()

	if err := s.teams.Update(ctx, team); err != nil {
		if strings.Contains(err.Error(), "slug already exists") {
			team.Slug = fmt.Sprintf("%s-%s", team.Slug, teamID.String()[:8])
			if err := s.teams.Update(ctx, team); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return team, nil
}

func (s *Service) ListMembers(ctx context.Context, teamID, userID uuid.UUID) ([]MemberWithUser, error) {
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.ListMembers(ctx, teamID)
}

func (s *Service) AddMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole Role, email string, role Role) (*MemberWithUser, error) {
	if !canManageTeam(actorRole) {
		return nil, ErrForbidden
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, actorID); err != nil {
		return nil, err
	}
	if err := validateAssignableRole(role); err != nil {
		return nil, err
	}

	found, err := s.users.FindByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if _, err := s.teams.FindMemberByUser(ctx, teamID, found.ID); err == nil {
		return nil, ErrAlreadyMember
	} else if !errors.Is(err, ErrMemberNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	member := &Member{
		ID:        uuid.New(),
		TeamID:    teamID,
		UserID:    found.ID,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.teams.CreateMember(ctx, member); err != nil {
		return nil, err
	}

	return &MemberWithUser{
		ID:        member.ID,
		UserID:    found.ID,
		Email:     found.Email,
		Name:      found.Name,
		Role:      role,
		CreatedAt: now,
	}, nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, teamID, actorID uuid.UUID, actorRole Role, memberID uuid.UUID, newRole Role) (*MemberWithUser, error) {
	if !canManageTeam(actorRole) {
		return nil, ErrForbidden
	}
	if err := validateAssignableRole(newRole); err != nil {
		return nil, err
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err != nil {
		return nil, err
	}
	if target.Role == RoleOwner {
		return nil, ErrCannotModifyOwner
	}

	if err := s.teams.UpdateMemberRole(ctx, teamID, memberID, newRole); err != nil {
		return nil, err
	}

	members, err := s.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if m.ID == memberID {
			return &m, nil
		}
	}
	return nil, ErrMemberNotFound
}

func (s *Service) RemoveMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole Role, memberID uuid.UUID) error {
	if !canManageTeam(actorRole) {
		return ErrForbidden
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err != nil {
		return err
	}
	if target.Role == RoleOwner {
		return ErrCannotModifyOwner
	}

	return s.teams.SoftDeleteMember(ctx, teamID, memberID)
}

func canManageTeam(role Role) bool {
	return role == RoleOwner || role == RoleAdmin
}

func validateAssignableRole(role Role) error {
	switch role {
	case RoleAdmin, RoleAgent:
		return nil
	default:
		return ErrInvalidRole
	}
}
