package workspace

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const totalReadinessChecks = 5

type ReadinessCheck struct {
	Key       string `json:"key"`
	Completed bool   `json:"completed"`
}

type ReadinessResult struct {
	Score    int              `json:"score"`
	Status   string           `json:"status"`
	Checks   []ReadinessCheck `json:"checks"`
}

type readinessDeps struct {
	channels  channel.Repository
	templates template.Repository
	contacts  contact.Repository
	teams     team.Repository
	campaigns campaign.Repository
	snapshots SnapshotRepository
}

func (s *Service) readinessDeps() readinessDeps {
	return readinessDeps{
		channels:  s.channels,
		templates: s.templates,
		contacts:  s.contacts,
		teams:     s.teams,
		campaigns: s.campaigns,
		snapshots: s.snapshots,
	}
}

func (s *Service) GetReadiness(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*ReadinessResult, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return computeReadiness(ctx, s.readinessDeps(), teamID)
}

func computeReadiness(ctx context.Context, deps readinessDeps, teamID uuid.UUID) (*ReadinessResult, error) {
	checks, err := evaluateReadinessChecks(ctx, deps, teamID)
	if err != nil {
		return nil, err
	}
	completed := 0
	for _, check := range checks {
		if check.Completed {
			completed++
		}
	}
	score := calculateReadinessScore(completed, totalReadinessChecks)
	status := resolveReadinessStatus(score)
	result := &ReadinessResult{
		Score:  score,
		Status: status,
		Checks: checks,
	}
	if deps.snapshots != nil {
		var onboarding *OnboardingState
		if existing, err := deps.snapshots.FindByTeam(ctx, teamID); err == nil {
			onboarding = existing.Onboarding
		}
		_ = deps.snapshots.Upsert(ctx, &ReadinessSnapshot{
			TeamID:         teamID,
			Score:          score,
			CompletedCount: completed,
			TotalCount:     totalReadinessChecks,
			Status:         status,
			Checks:         checks,
			Onboarding:     onboarding,
			ComputedAt:     time.Now().UTC(),
		})
	}
	return result, nil
}

func evaluateReadinessChecks(ctx context.Context, deps readinessDeps, teamID uuid.UUID) ([]ReadinessCheck, error) {
	connected := channel.StatusConnected
	channelPage, err := deps.channels.List(ctx, teamID, channel.ListFilter{Status: &connected}, repository.PageParams{Page: 1, PageSize: 1})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	templatePage, err := deps.templates.List(ctx, teamID, template.ListFilter{}, repository.PageParams{Page: 1, PageSize: 1})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	contactCount, err := deps.contacts.CountByTeam(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	members, err := deps.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	campaignPage, err := deps.campaigns.List(ctx, teamID, campaign.ListFilter{}, repository.PageParams{Page: 1, PageSize: 1})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	return []ReadinessCheck{
		{Key: "channel_connected", Completed: channelPage.Pagination.Total > 0},
		{Key: "template_synced", Completed: templatePage.Pagination.Total > 0},
		{Key: "contacts_available", Completed: contactCount > 0},
		{Key: "team_members_added", Completed: len(members) > 1},
		{Key: "campaign_created", Completed: campaignPage.Pagination.Total > 0},
	}, nil
}
