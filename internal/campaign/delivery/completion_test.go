package delivery_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type completionRepo struct {
	nonTerm   int
	completed int
	lastAt    time.Time
}

func (r *completionRepo) CountNonTerminalRecipients(context.Context, uuid.UUID) (int, error) {
	return r.nonTerm, nil
}
func (r *completionRepo) CompleteCampaign(_ context.Context, _, _ uuid.UUID, at time.Time) error {
	if r.completed > 0 {
		return repository.ErrNotFound
	}
	r.completed++
	r.lastAt = at
	return nil
}

func (r *completionRepo) Create(context.Context, *campaign.Campaign, []campaign.Audience) error {
	return nil
}
func (r *completionRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*campaign.Campaign, error) {
	return nil, repository.ErrNotFound
}
func (r *completionRepo) List(context.Context, uuid.UUID, campaign.ListFilter, repository.PageParams) (repository.Page[campaign.Campaign], error) {
	return repository.Page[campaign.Campaign]{}, nil
}
func (r *completionRepo) Update(context.Context, *campaign.Campaign) error { return nil }
func (r *completionRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, campaign.Status) error {
	return nil
}
func (r *completionRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *completionRepo) ListAudiences(context.Context, uuid.UUID) ([]campaign.Audience, error) {
	return nil, nil
}
func (r *completionRepo) CreateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (r *completionRepo) UpdateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (r *completionRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*campaign.BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (r *completionRepo) CreateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *completionRepo) UpdateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *completionRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]campaign.BroadcastBatch, error) {
	return nil, nil
}
func (r *completionRepo) CreateAudienceSnapshot(context.Context, *campaign.AudienceSnapshot) error {
	return nil
}
func (r *completionRepo) BulkCreateRecipients(context.Context, []campaign.Recipient, time.Time) error {
	return nil
}
func (r *completionRepo) UpdateRecipientStatus(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, *string, *string) error {
	return nil
}
func (r *completionRepo) UpdateRecipientDelivery(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, campaign.RecipientDeliveryUpdate) error {
	return nil
}
func (r *completionRepo) FindRecipientByWAMID(context.Context, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *completionRepo) FindRecipientByWAMIDForTeam(context.Context, uuid.UUID, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *completionRepo) ApplyCounterDelta(context.Context, uuid.UUID, campaign.CounterDelta) error {
	return nil
}
func (r *completionRepo) AggregateRecipientStatusCounts(context.Context, uuid.UUID) (campaign.RecipientStatusCounts, error) {
	return campaign.RecipientStatusCounts{}, nil
}
func (r *completionRepo) SetCampaignCounters(context.Context, uuid.UUID, campaign.RecipientStatusCounts) error {
	return nil
}
func (r *completionRepo) RepairCampaignCounters(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (r *completionRepo) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *completionRepo) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *completionRepo) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func TestCompletionMonitorCompletesOnce(t *testing.T) {
	repo := &completionRepo{nonTerm: 1}
	monitor := delivery.NewCompletionMonitor(repo, nil, nil)
	teamID := uuid.New()
	campaignID := uuid.New()
	at := time.Now().UTC()

	ok, err := monitor.Check(context.Background(), teamID, campaignID, at)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("should not complete while recipients remain")
	}

	repo.nonTerm = 0
	ok, err = monitor.Check(context.Background(), teamID, campaignID, at)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || repo.completed != 1 {
		t.Fatalf("expected single completion, completed=%d", repo.completed)
	}
	if repo.lastAt.IsZero() {
		t.Fatal("expected completed_at timestamp")
	}

	ok, err = monitor.Check(context.Background(), teamID, campaignID, at)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("second check should be idempotent")
	}
	if repo.completed != 1 {
		t.Fatalf("expected completion only once, got %d", repo.completed)
	}
}
