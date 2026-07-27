package template

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (TemplateService) manages Meta-synced WhatsApp templates.
type Service struct {
	templates Repository
	channels  channel.Repository
	teams     team.Repository
	rbac      *rbac.Service
	sync      *SyncService
	query     *QueryService
}

func NewService(
	templates Repository,
	channels channel.Repository,
	teams team.Repository,
	rbacSvc *rbac.Service,
	syncSvc *SyncService,
) *Service {
	return &Service{
		templates: templates,
		channels:  channels,
		teams:     teams,
		rbac:      rbacSvc,
		sync:      syncSvc,
		query:     NewQueryService(templates),
	}
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
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, templateID uuid.UUID) (*Template, error) {
	if err := s.authorize(actorRole, rbac.PermTemplateView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	tmpl, err := s.templates.FindByID(ctx, teamID, templateID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return tmpl, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Template], error) {
	if err := s.authorize(actorRole, rbac.PermTemplateView); err != nil {
		return repository.Page[Template]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Template]{}, err
	}
	result, err := s.query.Search(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Template]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

// UpsertFromSync persists a template synced from Meta (called by sync worker/handler later).
func (s *Service) UpsertFromSync(ctx context.Context, teamID uuid.UUID, actorRole string, tmpl *Template) error {
	if err := s.authorize(actorRole, rbac.PermTemplateSync); err != nil {
		return err
	}
	if _, err := s.channels.FindByID(ctx, teamID, tmpl.ChannelID); err != nil {
		return serviceerrors.MapRepository(err)
	}
	tmpl.TeamID = teamID
	tmpl.UpdatedAt = time.Now().UTC()
	now := time.Now().UTC()
	if tmpl.CreatedAt.IsZero() {
		tmpl.CreatedAt = now
	}
	if tmpl.LastSyncAt == nil {
		tmpl.LastSyncAt = &now
	}
	return serviceerrors.MapRepository(s.templates.UpsertFromSync(ctx, tmpl))
}

// ValidateForCampaign ensures the template exists, is approved, and belongs to the channel.
func (s *Service) ValidateForCampaign(ctx context.Context, teamID, channelID, templateID uuid.UUID) error {
	tmpl, err := s.templates.FindByID(ctx, teamID, templateID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if tmpl.ChannelID != channelID {
		return serviceerrors.NewValidation("template_id", "template does not belong to the selected channel")
	}
	if tmpl.Status != StatusApproved {
		return serviceerrors.NewValidation("template_id", "template must be approved by Meta")
	}
	name := strings.TrimSpace(tmpl.Name)
	if name == "" {
		return serviceerrors.NewValidation("template_id", "template name is required")
	}
	return nil
}

func (s *Service) SyncFromMeta(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*SyncResult, error) {
	if err := s.authorize(actorRole, rbac.PermTemplateSync); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.sync == nil {
		return nil, serviceerrors.NewValidation("sync", "template sync is not configured")
	}
	return s.sync.SyncChannel(ctx, teamID, channelID)
}

func (s *Service) ListSyncHistory(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter SyncRunListFilter, page repository.PageParams) (repository.Page[SyncHistoryItem], error) {
	if err := s.authorize(actorRole, rbac.PermTemplateView); err != nil {
		return repository.Page[SyncHistoryItem]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[SyncHistoryItem]{}, err
	}
	if s.sync == nil || s.sync.SyncRuns() == nil {
		return repository.Page[SyncHistoryItem]{Items: []SyncHistoryItem{}, Pagination: repository.NewMeta(page.Page, page.PageSize, 0)}, nil
	}
	runs, err := s.sync.SyncRuns().List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[SyncHistoryItem]{}, serviceerrors.MapRepository(err)
	}
	items := make([]SyncHistoryItem, 0, len(runs.Items))
	for _, run := range runs.Items {
		items = append(items, s.toSyncHistoryItem(ctx, teamID, run))
	}
	return repository.Page[SyncHistoryItem]{Items: items, Pagination: runs.Pagination}, nil
}

func (s *Service) GetSyncHistory(ctx context.Context, teamID, userID uuid.UUID, actorRole string, runID uuid.UUID) (*SyncHistoryItem, error) {
	if err := s.authorize(actorRole, rbac.PermTemplateView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.sync == nil || s.sync.SyncRuns() == nil {
		return nil, serviceerrors.ErrNotFound
	}
	run, err := s.sync.SyncRuns().FindByID(ctx, teamID, runID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	item := s.toSyncHistoryItem(ctx, teamID, *run)
	return &item, nil
}

func (s *Service) GetSyncMetrics(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID *uuid.UUID) (*SyncMetrics, error) {
	if err := s.authorize(actorRole, rbac.PermTemplateView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.sync == nil || s.sync.SyncRuns() == nil {
		return &SyncMetrics{}, nil
	}
	if channelID != nil {
		return s.sync.SyncRuns().MetricsByChannel(ctx, teamID, *channelID)
	}
	return s.sync.SyncRuns().MetricsByTeam(ctx, teamID)
}

func (s *Service) toSyncHistoryItem(ctx context.Context, teamID uuid.UUID, run SyncRun) SyncHistoryItem {
	channelName := ""
	if ch, err := s.channels.FindByID(ctx, teamID, run.ChannelID); err == nil {
		channelName = ch.Name
	}
	return SyncHistoryItem{
		ID:          run.ID,
		ChannelID:   run.ChannelID,
		ChannelName: channelName,
		Status:      run.Status,
		StartedAt:   run.StartedAt,
		CompletedAt: run.CompletedAt,
		Created:     run.CreatedCount,
		Updated:     run.UpdatedCount,
		Removed:     run.RemovedCount,
		ErrorCount:  run.ErrorCount,
	}
}
