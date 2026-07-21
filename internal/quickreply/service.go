package quickreply

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

type Service struct {
	items Repository
	teams team.Repository
	rbac  *rbac.Service
}

func NewService(items Repository, teams team.Repository, rbacSvc ...*rbac.Service) *Service {
	svc := &Service{items: items, teams: teams}
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

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*QuickReply, error) {
	if err := s.authorize(actorRole, rbac.PermQuickReplyCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, serviceerrors.NewValidation("content", "is required")
	}
	now := time.Now().UTC()
	item := &QuickReply{
		ID:        uuid.New(),
		TeamID:    teamID,
		Category:  defaultCategory(input.Category),
		Content:   content,
		Text:      content,
		CreatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.items.Create(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID) (*QuickReply, error) {
	if err := s.authorize(actorRole, rbac.PermQuickReplyView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.items.FindByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[QuickReply], error) {
	if err := s.authorize(actorRole, rbac.PermQuickReplyView); err != nil {
		return repository.Page[QuickReply]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[QuickReply]{}, err
	}
	result, err := s.items.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[QuickReply]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID, input UpdateInput) (*QuickReply, error) {
	if err := s.authorize(actorRole, rbac.PermQuickReplyUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.items.FindByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Category != nil {
		item.Category = defaultCategory(*input.Category)
	}
	if input.Content != nil {
		content := strings.TrimSpace(*input.Content)
		if content == "" {
			return nil, serviceerrors.NewValidation("content", "is required")
		}
		item.Content = content
		item.Text = content
	}
	item.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.items.Update(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermQuickReplyDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.items.SoftDelete(ctx, teamID, id))
}

func defaultCategory(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "General"
	}
	return category
}
