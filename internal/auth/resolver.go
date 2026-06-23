package auth

import (
	"context"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// ActorResolver builds a workspace Actor from validated JWT claims.
type ActorResolver interface {
	Resolve(ctx context.Context, claims *jwtmanager.Claims) (Actor, error)
}

type resolver struct {
	teams team.Repository
	rbac  *rbac.Service
}

// NewResolver returns an ActorResolver backed by team membership and RBAC cache.
func NewResolver(teams team.Repository, rbacSvc *rbac.Service) ActorResolver {
	return &resolver{teams: teams, rbac: rbacSvc}
}

func (r *resolver) Resolve(ctx context.Context, claims *jwtmanager.Claims) (Actor, error) {
	if claims == nil {
		return Actor{}, serviceerrors.ErrForbidden
	}

	member, err := r.teams.FindMemberByUser(ctx, claims.TeamID, claims.UserID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return Actor{}, serviceerrors.ErrForbidden
		}
		return Actor{}, err
	}
	if member.DeletedAt != nil || member.Status != team.MemberStatusActive {
		return Actor{}, serviceerrors.ErrForbidden
	}

	roleName := strings.TrimSpace(string(member.Role))
	if roleName == "" {
		roleName = strings.TrimSpace(claims.Role)
	}

	actor := Actor{
		UserID:      claims.UserID,
		TeamID:      claims.TeamID,
		Role:        roleName,
		RoleID:      member.RoleID,
		Permissions: r.rbac.PermissionsForRole(roleName),
		Email:       claims.Email,
		MemberID:    member.ID,
	}
	if err := actor.Validate(); err != nil {
		return Actor{}, serviceerrors.ErrForbidden
	}
	return actor, nil
}
