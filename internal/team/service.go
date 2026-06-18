package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
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
	rbac  *rbac.Service
}

func NewService(teams Repository, users user.Repository, rbacService *rbac.Service) *Service {
	return &Service{teams: teams, users: users, rbac: rbacService}
}

func (s *Service) authorize(roleName, permission string) error {
	if s.rbac != nil && s.rbac.Require(roleName, permission) {
		return nil
	}
	return ErrForbidden
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID) (*Team, error) {
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.FindByID(ctx, teamID)
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, name string) (*Team, error) {
	if err := s.authorize(actorRole, rbac.PermWorkspaceManage); err != nil {
		return nil, err
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

func (s *Service) ListUsers(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListMembersFilter) ([]MemberWithUser, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.ListMembersFiltered(ctx, teamID, filter)
}

func (s *Service) AddMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, email string, role Role) (*MemberWithUser, error) {
	if err := s.authorize(actorRole, rbac.PermUserInvite); err != nil {
		return nil, err
	}
	return s.addMemberInternal(ctx, teamID, actorID, email, role)
}

func (s *Service) InviteUser(ctx context.Context, teamID, actorID uuid.UUID, actorRole, name, email string, role Role) (*Invitation, error) {
	if err := s.authorize(actorRole, rbac.PermUserInvite); err != nil {
		return nil, err
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, actorID); err != nil {
		return nil, err
	}
	if err := validateAssignableRole(role); err != nil {
		return nil, err
	}

	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	roleID, err := s.teams.FindRoleIDByName(ctx, string(role))
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	inv := &Invitation{
		ID:        uuid.New(),
		TeamID:    teamID,
		Email:     email,
		Name:      name,
		RoleID:    roleID,
		Role:      role,
		Status:    "pending",
		InvitedBy: actorID,
		CreatedAt: now,
	}
	if err := s.teams.CreateInvitation(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Service) addMemberInternal(ctx context.Context, teamID, actorID uuid.UUID, email string, role Role) (*MemberWithUser, error) {
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

	roleID, err := s.teams.FindRoleIDByName(ctx, string(role))
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	member := &Member{
		ID:        uuid.New(),
		TeamID:    teamID,
		UserID:    found.ID,
		Role:      role,
		RoleID:    roleID,
		Status:    MemberStatusActive,
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
		RoleID:    roleID,
		Status:    MemberStatusActive,
		CreatedAt: now,
	}, nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, memberID uuid.UUID, newRole Role) (*MemberWithUser, error) {
	if err := s.authorize(actorRole, rbac.PermUserUpdate); err != nil {
		return nil, err
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

	roleID, err := s.teams.FindRoleIDByName(ctx, string(newRole))
	if err != nil {
		return nil, err
	}

	if err := s.teams.UpdateMemberRole(ctx, teamID, memberID, newRole, roleID); err != nil {
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

func (s *Service) DeactivateMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, memberID uuid.UUID) (*MemberWithUser, error) {
	if err := s.authorize(actorRole, rbac.PermUserUpdate); err != nil {
		return nil, err
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err != nil {
		return nil, err
	}
	if target.Role == RoleOwner {
		return nil, ErrCannotModifyOwner
	}

	if err := s.teams.UpdateMemberStatus(ctx, teamID, memberID, MemberStatusInactive); err != nil {
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

func (s *Service) RemoveMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, memberID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermUserDelete); err != nil {
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

func (s *Service) GetWorkspace(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*WorkspaceDetail, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.GetWorkspaceDetail(ctx, teamID)
}

func (s *Service) CreateOwnerMember(ctx context.Context, teamID, userID uuid.UUID) error {
	roleID, err := s.teams.FindRoleIDByName(ctx, string(RoleOwner))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.teams.CreateMember(ctx, &Member{
		ID:        uuid.New(),
		TeamID:    teamID,
		UserID:    userID,
		Role:      RoleOwner,
		RoleID:    roleID,
		Status:    MemberStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func validateAssignableRole(role Role) error {
	switch role {
	case RoleAdmin, RoleSupervisor, RoleAgent:
		return nil
	default:
		return ErrInvalidRole
	}
}
