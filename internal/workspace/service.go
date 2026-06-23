package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
	"github.com/omnichannel/omni-chan-v2/pkg/slug"
)

// Service (WorkspaceService) manages workspace settings.
type Service struct {
	teams     team.Repository
	quota     *quota.Service
	rbac      *rbac.Service
	channels  channel.Repository
	templates template.Repository
	contacts  contact.Repository
	campaigns campaign.Repository
	snapshots SnapshotRepository
}

func NewService(
	teams team.Repository,
	quotaSvc *quota.Service,
	rbacSvc *rbac.Service,
	channels channel.Repository,
	templates template.Repository,
	contacts contact.Repository,
	campaigns campaign.Repository,
	snapshots SnapshotRepository,
) *Service {
	return &Service{
		teams:     teams,
		quota:     quotaSvc,
		rbac:      rbacSvc,
		channels:  channels,
		templates: templates,
		contacts:  contacts,
		campaigns: campaigns,
		snapshots: snapshots,
	}
}

type UpdateInput struct {
	Name     *string
	Timezone *string
	Country  *string
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, team.ErrMemberNotFound) {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*team.Team, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, team.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *Service) GetDetail(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*team.WorkspaceDetail, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	detail, err := s.teams.GetWorkspaceDetail(ctx, teamID)
	if err != nil {
		if errors.Is(err, team.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, err
	}
	return detail, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input UpdateInput) (*team.Team, error) {
	if err := s.authorize(actorRole, rbac.PermWorkspaceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, team.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, err
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		t.Name = name
		t.Slug = slug.FromName(name)
	}
	if input.Timezone != nil {
		tz := strings.TrimSpace(*input.Timezone)
		if tz == "" {
			return nil, serviceerrors.NewValidation("timezone", "is required")
		}
		t.Timezone = tz
	}
	if input.Country != nil {
		cc := strings.ToUpper(strings.TrimSpace(*input.Country))
		if len(cc) != 2 {
			return nil, serviceerrors.NewValidation("country", "must be a 2-letter ISO code")
		}
		t.Country = cc
	}
	t.UpdatedAt = time.Now().UTC()

	if err := s.teams.Update(ctx, t); err != nil {
		if strings.Contains(err.Error(), "slug already exists") {
			t.Slug = fmt.Sprintf("%s-%s", t.Slug, teamID.String()[:8])
			if err := s.teams.Update(ctx, t); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return t, nil
}

func (s *Service) GetQuotas(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*quota.Snapshot, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.quota.Snapshot(ctx, teamID)
}

// IsOwner reports whether userID owns the workspace.
func (s *Service) IsOwner(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, team.ErrNotFound) {
			return false, serviceerrors.ErrNotFound
		}
		return false, err
	}
	if t.OwnerID != nil && *t.OwnerID == userID {
		return true, nil
	}
	member, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, team.ErrMemberNotFound) {
			return false, nil
		}
		return false, err
	}
	return member.Role == team.RoleOwner, nil
}
