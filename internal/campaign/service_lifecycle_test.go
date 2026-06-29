package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type lifecycleRepo struct {
	campaign   *Campaign
	job        *BroadcastJob
	outbox     [][]byte
	transition int
	cancelled  bool
}

func (r *lifecycleRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*Campaign, error) {
	if r.campaign == nil || r.campaign.ID != id || r.campaign.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	cp := *r.campaign
	return &cp, nil
}
func (r *lifecycleRepo) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	r.transition++
	r.campaign.Status = StatusQueued
	return nil
}
func (r *lifecycleRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*BroadcastJob, error) {
	if r.job == nil {
		return nil, repository.ErrNotFound
	}
	cp := *r.job
	return &cp, nil
}
func (r *lifecycleRepo) CreateJob(_ context.Context, job *BroadcastJob) error {
	r.job = job
	return nil
}
func (r *lifecycleRepo) InsertOutbox(_ context.Context, _ string, payload []byte) error {
	r.outbox = append(r.outbox, payload)
	return nil
}
func (r *lifecycleRepo) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	r.cancelled = true
	r.campaign.Status = StatusCancelled
	return nil
}

func (r *lifecycleRepo) Create(context.Context, *Campaign, []Audience) error               { return nil }
func (r *lifecycleRepo) List(context.Context, uuid.UUID, ListFilter, repository.PageParams) (repository.Page[Campaign], error) {
	return repository.Page[Campaign]{}, nil
}
func (r *lifecycleRepo) Update(context.Context, *Campaign) error                           { return nil }
func (r *lifecycleRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, Status) error  { return nil }
func (r *lifecycleRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error            { return nil }
func (r *lifecycleRepo) ListAudiences(context.Context, uuid.UUID) ([]Audience, error) { return nil, nil }
func (r *lifecycleRepo) LoadListEnrichment(context.Context, uuid.UUID, []Campaign) (map[uuid.UUID]ListEnrichment, error) {
	return map[uuid.UUID]ListEnrichment{}, nil
}
func (r *lifecycleRepo) CreateBatch(context.Context, *BroadcastBatch) error                { return nil }
func (r *lifecycleRepo) UpdateBatch(context.Context, *BroadcastBatch) error                { return nil }
func (r *lifecycleRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]BroadcastBatch, error) {
	return nil, nil
}
func (r *lifecycleRepo) CreateAudienceSnapshot(context.Context, *AudienceSnapshot) error { return nil }
func (r *lifecycleRepo) BulkCreateRecipients(context.Context, []Recipient, time.Time) error {
	return nil
}
func (r *lifecycleRepo) UpdateRecipientStatus(context.Context, uuid.UUID, RecipientStatus, RecipientStatus, *string, *string) error {
	return nil
}
func (r *lifecycleRepo) UpdateRecipientDelivery(context.Context, uuid.UUID, RecipientStatus, RecipientStatus, RecipientDeliveryUpdate) error {
	return nil
}
func (r *lifecycleRepo) FindRecipientByWAMID(context.Context, string) (*Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *lifecycleRepo) FindRecipientByWAMIDForTeam(context.Context, uuid.UUID, string) (*Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *lifecycleRepo) ApplyCounterDelta(context.Context, uuid.UUID, CounterDelta) error { return nil }
func (r *lifecycleRepo) CountNonTerminalRecipients(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *lifecycleRepo) CompleteCampaign(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (r *lifecycleRepo) AggregateRecipientStatusCounts(context.Context, uuid.UUID) (RecipientStatusCounts, error) {
	return RecipientStatusCounts{}, nil
}
func (r *lifecycleRepo) SetCampaignCounters(context.Context, uuid.UUID, RecipientStatusCounts) error {
	return nil
}
func (r *lifecycleRepo) RepairCampaignCounters(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (r *lifecycleRepo) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *lifecycleRepo) UpdateJob(context.Context, *BroadcastJob) error { return nil }

type lifecycleRbacRepo struct{}

func (lifecycleRbacRepo) LoadRolePermissions(context.Context) (map[string]map[string]bool, error) {
	return map[string]map[string]bool{
		"owner": {
			rbac.PermCampaignSend:   true,
			rbac.PermCampaignCancel: true,
		},
	}, nil
}
func (lifecycleRbacRepo) ListRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (lifecycleRbacRepo) FindRoleByID(context.Context, uuid.UUID) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (lifecycleRbacRepo) FindRoleByName(context.Context, string) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (lifecycleRbacRepo) ListPermissions(context.Context) ([]rbac.Permission, error) { return nil, nil }
func (lifecycleRbacRepo) ListPermissionsByRoleID(context.Context, uuid.UUID) ([]rbac.Permission, error) {
	return nil, nil
}
func (lifecycleRbacRepo) GetPermissionMatrix(context.Context) ([]rbac.PermissionMatrixRow, error) {
	return nil, nil
}

func lifecycleTestRBAC(t *testing.T) *rbac.Service {
	t.Helper()
	svc := rbac.NewService(lifecycleRbacRepo{})
	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache: %v", err)
	}
	return svc
}

func TestServiceSendQueuesCampaign(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	campaignID := uuid.New()
	channelID := uuid.New()
	jobID := uuid.New()
	repo := &lifecycleRepo{
		campaign: &Campaign{
			ID: campaignID, TeamID: teamID, ChannelID: channelID, Status: StatusDraft,
		},
		job: &BroadcastJob{ID: jobID, CampaignID: campaignID, TeamID: teamID, ChannelID: channelID, Status: JobPending},
	}
	svc := NewService(repo, nil, &createTestTeamRepo{member: &team.Member{}}, nil, nil, nil, lifecycleTestRBAC(t))

	result, err := svc.Send(context.Background(), teamID, userID, "owner", campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Campaign.Status != StatusQueued {
		t.Fatalf("expected queued, got %s", result.Campaign.Status)
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("expected outbox enqueue, got %d", len(repo.outbox))
	}
}

func TestServiceSendRejectsCompleted(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	repo := &lifecycleRepo{
		campaign: &Campaign{ID: campaignID, TeamID: teamID, Status: StatusCompleted},
	}
	svc := NewService(repo, nil, &createTestTeamRepo{member: &team.Member{}}, nil, nil, nil, lifecycleTestRBAC(t))

	_, err := svc.Send(context.Background(), teamID, uuid.New(), "owner", campaignID)
	if err == nil || !serviceerrors.IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestServiceCancelIdempotent(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	repo := &lifecycleRepo{
		campaign: &Campaign{ID: campaignID, TeamID: teamID, Status: StatusCancelled},
	}
	svc := NewService(repo, nil, &createTestTeamRepo{member: &team.Member{}}, nil, nil, nil, lifecycleTestRBAC(t))

	camp, err := svc.Cancel(context.Background(), teamID, uuid.New(), "owner", campaignID, "user request")
	if err != nil {
		t.Fatal(err)
	}
	if camp.Status != StatusCancelled {
		t.Fatal("expected cancelled")
	}
	if repo.cancelled {
		t.Fatal("cancel should be idempotent without repo call")
	}
}

func TestServiceCancelRejectsCompleted(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	repo := &lifecycleRepo{
		campaign: &Campaign{ID: campaignID, TeamID: teamID, Status: StatusCompleted},
	}
	svc := NewService(repo, nil, &createTestTeamRepo{member: &team.Member{}}, nil, nil, nil, lifecycleTestRBAC(t))

	_, err := svc.Cancel(context.Background(), teamID, uuid.New(), "owner", campaignID, "")
	if err != serviceerrors.ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}
