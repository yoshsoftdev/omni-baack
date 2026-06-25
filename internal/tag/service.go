package tag

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (TagService) manages contact tags.
type Service struct {
	tags  Repository
	teams team.Repository
	rbac  *rbac.Service
}

func NewService(tags Repository, teams team.Repository, rbacSvc ...*rbac.Service) *Service {
	svc := &Service{tags: tags, teams: teams}
	if len(rbacSvc) > 0 {
		svc.rbac = rbacSvc[0]
	}
	return svc
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	if s.rbac == nil && role != "" {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) authorizeAny(role string, perms ...string) error {
	if s.rbac == nil && role != "" {
		return nil
	}
	if s.rbac != nil {
		for _, perm := range perms {
			if s.rbac.Require(role, perm) {
				return nil
			}
		}
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Tag, error) {
	if err := s.authorize(actorRole, rbac.PermTagCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}
	now := time.Now().UTC()
	t := &Tag{
		ID:        uuid.New(),
		TeamID:    teamID,
		Name:      name,
		Color:     defaultColor(input.Color),
		CreatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		if desc != "" {
			t.Description = &desc
		}
	}
	if err := serviceerrors.MapRepository(s.tags.Create(ctx, t)); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, tagID uuid.UUID) (*Tag, error) {
	if err := s.authorize(actorRole, rbac.PermTagView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	t, err := s.tags.FindByID(ctx, teamID, tagID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return t, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Tag], error) {
	if err := s.authorize(actorRole, rbac.PermTagView); err != nil {
		return repository.Page[Tag]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Tag]{}, err
	}
	result, err := s.tags.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Tag]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, tagID uuid.UUID, input UpdateInput) (*Tag, error) {
	if err := s.authorize(actorRole, rbac.PermTagUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	t, err := s.tags.FindByID(ctx, teamID, tagID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		t.Name = name
	}
	if input.Color != nil {
		t.Color = defaultColor(*input.Color)
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		if desc == "" {
			t.Description = nil
		} else {
			t.Description = &desc
		}
	}
	t.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.tags.Update(ctx, t)); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, tagID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermTagDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.tags.SoftDelete(ctx, teamID, tagID))
}

func (s *Service) Assign(ctx context.Context, teamID, userID uuid.UUID, actorRole string, tagID, contactID uuid.UUID) error {
	// Tag assignment is treated as a collaborative inbox action.
	// Any valid team member can attach tags to contacts.
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.tags.AssignContact(ctx, teamID, tagID, contactID))
}

func (s *Service) Unassign(ctx context.Context, teamID, userID uuid.UUID, actorRole string, tagID, contactID uuid.UUID) error {
	// Same policy as Assign: team membership is sufficient.
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.tags.UnassignContact(ctx, teamID, tagID, contactID))
}

func defaultColor(color string) string {
	c := strings.TrimSpace(color)
	if c == "" {
		return "#6366f1"
	}
	return c
}
