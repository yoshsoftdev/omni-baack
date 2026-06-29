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
	"github.com/omnichannel/omni-chan-v2/pkg/planlimits"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
	"github.com/omnichannel/omni-chan-v2/pkg/slug"
)

var (
	ErrForbidden         = errors.New("insufficient permissions")
	ErrInvalidRole       = errors.New("invalid role")
	ErrCannotModifyOwner = errors.New("cannot modify team owner")
	ErrUserNotFound      = errors.New("user not found, they must register first")
)

const invitationTTL = 7 * 24 * time.Hour

// MemberLimiter checks workspace member quotas (implemented by quota.Service).
type MemberLimiter interface {
	AssertMemberHeadroom(ctx context.Context, teamID uuid.UUID) error
}

// Service (TeamService) manages team membership and invitations.
type Service struct {
	teams   Repository
	users   user.Repository
	limiter MemberLimiter
	rbac    *rbac.Service
}

func NewService(teams Repository, users user.Repository, limiter MemberLimiter, rbacService *rbac.Service) *Service {
	return &Service{teams: teams, users: users, limiter: limiter, rbac: rbacService}
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
	team.Slug = slug.FromName(name)
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
	members, err := s.teams.ListMembersFiltered(ctx, teamID, filter)
	if err != nil {
		return nil, err
	}
	invitations, err := s.teams.ListInvitations(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return mergeInvitedUsers(members, invitations, filter), nil
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
	if s.limiter != nil {
		if err := s.limiter.AssertMemberHeadroom(ctx, teamID); err != nil {
			return nil, err
		}
	} else if err := s.assertMemberHeadroom(ctx, teamID); err != nil {
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

func (s *Service) GetInvitationPublic(ctx context.Context, invitationID uuid.UUID) (*InvitationDetail, error) {
	detail, err := s.teams.GetPendingInvitation(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(detail.CreatedAt.Add(invitationTTL)) {
		return nil, ErrInvitationExpired
	}
	return detail, nil
}

func (s *Service) addMemberInternal(ctx context.Context, teamID, actorID uuid.UUID, email string, role Role) (*MemberWithUser, error) {
	if _, err := s.teams.FindMemberByUser(ctx, teamID, actorID); err != nil {
		return nil, err
	}
	if s.limiter != nil {
		if err := s.limiter.AssertMemberHeadroom(ctx, teamID); err != nil {
			return nil, err
		}
	} else if err := s.assertMemberHeadroom(ctx, teamID); err != nil {
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

	roleID, err := s.teams.FindRoleIDByName(ctx, string(newRole))
	if err != nil {
		return nil, err
	}
	if newRole == RoleOwner {
		return nil, serviceerrors.NewValidation("role", "cannot assign owner role")
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err == nil {
		if target.Role == RoleOwner {
			return nil, ErrCannotModifyOwner
		}
		if err := s.teams.UpdateMemberRole(ctx, teamID, memberID, newRole, roleID); err != nil {
			return nil, err
		}
		return s.findListedUser(ctx, teamID, memberID)
	}
	if !errors.Is(err, ErrMemberNotFound) {
		return nil, err
	}

	inv, err := s.teams.FindInvitationByID(ctx, teamID, memberID)
	if err != nil {
		return nil, err
	}
	if err := s.teams.UpdateInvitationRole(ctx, teamID, memberID, newRole, roleID); err != nil {
		return nil, err
	}
	return invitationToMember(inv, newRole, roleID), nil
}

func (s *Service) DeactivateMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, memberID uuid.UUID) (*MemberWithUser, error) {
	if err := s.authorize(actorRole, rbac.PermUserUpdate); err != nil {
		return nil, err
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err == nil {
		if target.Role == RoleOwner {
			return nil, ErrCannotModifyOwner
		}
		if err := s.teams.UpdateMemberStatus(ctx, teamID, memberID, MemberStatusInactive); err != nil {
			return nil, err
		}
		return s.findListedUser(ctx, teamID, memberID)
	}
	if !errors.Is(err, ErrMemberNotFound) {
		return nil, err
	}

	inv, err := s.teams.FindInvitationByID(ctx, teamID, memberID)
	if err != nil {
		return nil, err
	}
	if err := s.teams.SoftDeleteInvitation(ctx, teamID, memberID); err != nil {
		return nil, err
	}
	member := invitationToMember(inv, inv.Role, inv.RoleID)
	member.Status = MemberStatusInactive
	return member, nil
}

func (s *Service) RemoveMember(ctx context.Context, teamID, actorID uuid.UUID, actorRole string, memberID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermUserDelete); err != nil {
		return ErrForbidden
	}

	target, err := s.teams.FindMemberByID(ctx, teamID, memberID)
	if err == nil {
		if target.Role == RoleOwner {
			return ErrCannotModifyOwner
		}
		return s.teams.SoftDeleteMember(ctx, teamID, memberID)
	}
	if !errors.Is(err, ErrMemberNotFound) {
		return err
	}

	if _, err := s.teams.FindInvitationByID(ctx, teamID, memberID); err != nil {
		return err
	}
	return s.teams.SoftDeleteInvitation(ctx, teamID, memberID)
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

func (s *Service) assertMemberHeadroom(ctx context.Context, teamID uuid.UUID) error {
	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		return err
	}
	limit := planlimits.MemberLimit(string(t.PlanType))
	if limit == 0 {
		return nil
	}
	members, err := s.teams.ListMembers(ctx, teamID)
	if err != nil {
		return err
	}
	if len(members) >= limit {
		return serviceerrors.ErrQuotaExceeded
	}
	return nil
}

func mergeInvitedUsers(members []MemberWithUser, invitations []Invitation, filter ListMembersFilter) []MemberWithUser {
	memberEmails := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberEmails[strings.ToLower(member.Email)] = struct{}{}
	}

	search := strings.ToLower(strings.TrimSpace(filter.Search))
	for _, invitation := range invitations {
		email := strings.ToLower(strings.TrimSpace(invitation.Email))
		if _, exists := memberEmails[email]; exists {
			continue
		}
		if filter.Role != "" && string(invitation.Role) != filter.Role {
			continue
		}
		if search != "" {
			name := strings.ToLower(invitation.Name)
			if !strings.Contains(name, search) && !strings.Contains(email, search) {
				continue
			}
		}
		members = append(members, MemberWithUser{
			ID:        invitation.ID,
			Email:     invitation.Email,
			Name:      invitation.Name,
			Role:      invitation.Role,
			RoleID:    invitation.RoleID,
			Status:    MemberStatusInvited,
			CreatedAt: invitation.CreatedAt,
		})
	}
	return members
}

func invitationToMember(invitation *Invitation, role Role, roleID uuid.UUID) *MemberWithUser {
	return &MemberWithUser{
		ID:        invitation.ID,
		Email:     invitation.Email,
		Name:      invitation.Name,
		Role:      role,
		RoleID:    roleID,
		Status:    MemberStatusInvited,
		CreatedAt: invitation.CreatedAt,
	}
}

func (s *Service) findListedUser(ctx context.Context, teamID, memberID uuid.UUID) (*MemberWithUser, error) {
	members, err := s.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.ID == memberID {
			return &member, nil
		}
	}
	return nil, ErrMemberNotFound
}
