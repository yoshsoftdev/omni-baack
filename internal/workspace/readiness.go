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

const totalReadinessChecks = 7

type templateSyncChecker interface {
	TeamRecentlySynced(ctx context.Context, teamID uuid.UUID) (bool, error)
}

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
	channels    channel.Repository
	templates   template.Repository
	contacts    contact.Repository
	teams       team.Repository
	campaigns   campaign.Repository
	snapshots   SnapshotRepository
	syncChecker templateSyncChecker
}

func (s *Service) readinessDeps() readinessDeps {
	return readinessDeps{
		channels:    s.channels,
		templates:   s.templates,
		contacts:    s.contacts,
		teams:       s.teams,
		campaigns:   s.campaigns,
		snapshots:   s.snapshots,
		syncChecker: s.syncChecker,
	}
}

func (s *Service) GetReadiness(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*ReadinessResult, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.snapshots != nil {
		if snap, err := s.snapshots.FindByTeam(ctx, teamID); err == nil {
			return readinessFromSnapshot(snap), nil
		}
	}
	return computeReadiness(ctx, s.readinessDeps(), teamID)
}

func (s *Service) RefreshReadiness(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*ReadinessResult, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	var onboarding *OnboardingState
	if s.snapshots != nil {
		if existing, err := s.snapshots.FindByTeam(ctx, teamID); err == nil {
			onboarding = existing.Onboarding
		}
	}
	return refreshReadinessSnapshot(ctx, s.readinessDeps(), teamID, onboarding)
}

func (s *Service) ReadinessScoreFromSnapshot(ctx context.Context, teamID uuid.UUID) int {
	if s.snapshots == nil {
		return 0
	}
	snap, err := s.snapshots.FindByTeam(ctx, teamID)
	if err != nil {
		return 0
	}
	return snap.Score
}

func readinessFromSnapshot(snap *ReadinessSnapshot) *ReadinessResult {
	return &ReadinessResult{
		Score:  snap.Score,
		Status: snap.Status,
		Checks: snap.Checks,
	}
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
	return &ReadinessResult{
		Score:  score,
		Status: status,
		Checks: checks,
	}, nil
}

func refreshReadinessSnapshot(ctx context.Context, deps readinessDeps, teamID uuid.UUID, onboarding *OnboardingState) (*ReadinessResult, error) {
	result, err := computeReadiness(ctx, deps, teamID)
	if err != nil {
		return nil, err
	}
	if deps.snapshots == nil {
		return result, nil
	}
	completed := countCompleted(result.Checks)
	if err := deps.snapshots.Upsert(ctx, &ReadinessSnapshot{
		TeamID:         teamID,
		Score:          result.Score,
		CompletedCount: completed,
		TotalCount:     totalReadinessChecks,
		Status:         result.Status,
		Checks:         result.Checks,
		Onboarding:     onboarding,
		ComputedAt:     time.Now().UTC(),
	}); err != nil {
		return nil, serviceerrors.MapRepository(err)
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
	metaReady, err := deps.channels.CountMetaReady(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	templatesRecentlySynced := false
	if deps.templates != nil {
		recent, err := deps.templates.HasRecentSync(ctx, teamID, 24*time.Hour)
		if err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		if !recent && deps.syncChecker != nil {
			recent, err = deps.syncChecker.TeamRecentlySynced(ctx, teamID)
			if err != nil {
				return nil, err
			}
		}
		templatesRecentlySynced = recent
	}

	return []ReadinessCheck{
		{Key: "meta_connected", Completed: metaReady > 0},
		{Key: "templates_recently_synced", Completed: templatesRecentlySynced},
		{Key: "channel_connected", Completed: channelPage.Pagination.Total > 0},
		{Key: "template_synced", Completed: templatePage.Pagination.Total > 0},
		{Key: "contacts_available", Completed: contactCount > 0},
		{Key: "team_members_added", Completed: len(members) > 1},
		{Key: "campaign_created", Completed: campaignPage.Pagination.Total > 0},
	}, nil
}
