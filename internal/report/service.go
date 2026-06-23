package report

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (ReportService) provides campaign reporting and dashboard aggregates.
type Service struct {
	reports Repository
	teams   team.Repository
	rbac    *rbac.Service
}

func NewService(reports Repository, teams team.Repository, rbacSvc *rbac.Service) *Service {
	return &Service{reports: reports, teams: teams, rbac: rbacSvc}
}

type DashboardInput struct {
	From      time.Time
	To        time.Time
	ChannelID *uuid.UUID
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

func (s *Service) GetDashboard(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input DashboardInput) (*DashboardStats, error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := validateDateRange(input.From, input.To); err != nil {
		return nil, err
	}
	stats, err := s.reports.AggregateDashboard(ctx, teamID, DateRange{From: input.From, To: input.To}, input.ChannelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return stats, nil
}

func (s *Service) ListDailyTrend(ctx context.Context, teamID, userID uuid.UUID, actorRole string, from, to time.Time) ([]DailyStat, error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := validateDateRange(from, to); err != nil {
		return nil, err
	}
	stats, err := s.reports.ListDailyStats(ctx, teamID, DateRange{From: from, To: to})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return stats, nil
}

func (s *Service) ListCampaignReports(ctx context.Context, teamID, userID uuid.UUID, actorRole string, from, to time.Time, page repository.PageParams) (repository.Page[CampaignFunnel], error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return repository.Page[CampaignFunnel]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[CampaignFunnel]{}, err
	}
	if err := validateDateRange(from, to); err != nil {
		return repository.Page[CampaignFunnel]{}, err
	}
	result, err := s.reports.ListCampaignFunnels(ctx, teamID, DateRange{From: from, To: to}, page)
	if err != nil {
		return repository.Page[CampaignFunnel]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) GetCampaignReport(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID) (*CampaignFunnel, error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	funnel, err := s.reports.GetCampaignFunnel(ctx, teamID, campaignID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return funnel, nil
}

func (s *Service) ListLinkClicks(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID, page repository.PageParams) (repository.Page[LinkClick], error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return repository.Page[LinkClick]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[LinkClick]{}, err
	}
	if _, err := s.reports.GetCampaignFunnel(ctx, teamID, campaignID); err != nil {
		return repository.Page[LinkClick]{}, serviceerrors.MapRepository(err)
	}
	result, err := s.reports.ListLinkClicks(ctx, teamID, campaignID, page)
	if err != nil {
		return repository.Page[LinkClick]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

// RecordLinkClick persists a tracked link click (webhook/redirect handler calls this later).
func (s *Service) RecordLinkClick(ctx context.Context, click *LinkClick) error {
	if click.ID == uuid.Nil {
		click.ID = uuid.New()
	}
	if click.ClickedAt.IsZero() {
		click.ClickedAt = time.Now().UTC()
	}
	if err := serviceerrors.MapRepository(s.reports.RecordLinkClick(ctx, click)); err != nil {
		return err
	}
	statDate := click.ClickedAt.UTC().Truncate(24 * time.Hour)
	return serviceerrors.MapRepository(s.reports.IncrementDailyStat(ctx, click.TeamID, statDate, DailyStatDelta{LinkClicks: 1}))
}

func validateDateRange(from, to time.Time) error {
	if from.IsZero() || to.IsZero() {
		return serviceerrors.NewValidation("from/to", "date range is required")
	}
	if to.Before(from) {
		return serviceerrors.NewValidation("to", "must be on or after from")
	}
	return nil
}
