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

type reconcileRepo struct {
	campaigns map[uuid.UUID]campaign.RecipientStatusCounts
	stored    map[uuid.UUID]campaign.RecipientStatusCounts
	repaired  []uuid.UUID
}

func newReconcileRepo() *reconcileRepo {
	return &reconcileRepo{
		campaigns: map[uuid.UUID]campaign.RecipientStatusCounts{},
		stored:    map[uuid.UUID]campaign.RecipientStatusCounts{},
	}
}

func (r *reconcileRepo) RepairCampaignCounters(_ context.Context, campaignID uuid.UUID) (bool, error) {
	counts := r.campaigns[campaignID]
	stored := r.stored[campaignID]
	if counts == stored {
		return false, nil
	}
	r.stored[campaignID] = counts
	r.repaired = append(r.repaired, campaignID)
	return true, nil
}

func (r *reconcileRepo) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(r.campaigns))
	for id := range r.campaigns {
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *reconcileRepo) Create(context.Context, *campaign.Campaign, []campaign.Audience) error {
	return nil
}
func (r *reconcileRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*campaign.Campaign, error) {
	return nil, repository.ErrNotFound
}
func (r *reconcileRepo) List(context.Context, uuid.UUID, campaign.ListFilter, repository.PageParams) (repository.Page[campaign.Campaign], error) {
	return repository.Page[campaign.Campaign]{}, nil
}
func (r *reconcileRepo) Update(context.Context, *campaign.Campaign) error { return nil }
func (r *reconcileRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, campaign.Status) error {
	return nil
}
func (r *reconcileRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *reconcileRepo) ListAudiences(context.Context, uuid.UUID) ([]campaign.Audience, error) {
	return nil, nil
}
func (r *reconcileRepo) CreateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (r *reconcileRepo) UpdateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (r *reconcileRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*campaign.BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (r *reconcileRepo) CreateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *reconcileRepo) UpdateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *reconcileRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]campaign.BroadcastBatch, error) {
	return nil, nil
}
func (r *reconcileRepo) CreateAudienceSnapshot(context.Context, *campaign.AudienceSnapshot) error {
	return nil
}
func (r *reconcileRepo) BulkCreateRecipients(context.Context, []campaign.Recipient, time.Time) error {
	return nil
}
func (r *reconcileRepo) UpdateRecipientStatus(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, *string, *string) error {
	return nil
}
func (r *reconcileRepo) UpdateRecipientDelivery(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, campaign.RecipientDeliveryUpdate) error {
	return nil
}
func (r *reconcileRepo) FindRecipientByWAMID(context.Context, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *reconcileRepo) FindRecipientByWAMIDForTeam(context.Context, uuid.UUID, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *reconcileRepo) ApplyCounterDelta(context.Context, uuid.UUID, campaign.CounterDelta) error {
	return nil
}
func (r *reconcileRepo) CountNonTerminalRecipients(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *reconcileRepo) CompleteCampaign(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (r *reconcileRepo) AggregateRecipientStatusCounts(_ context.Context, campaignID uuid.UUID) (campaign.RecipientStatusCounts, error) {
	return r.campaigns[campaignID], nil
}
func (r *reconcileRepo) SetCampaignCounters(_ context.Context, campaignID uuid.UUID, counts campaign.RecipientStatusCounts) error {
	r.stored[campaignID] = counts
	return nil
}
func (r *reconcileRepo) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *reconcileRepo) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func TestReconcilerRepairsDrift(t *testing.T) {
	id := uuid.New()
	repo := newReconcileRepo()
	repo.campaigns[id] = campaign.RecipientStatusCounts{Sent: 10, Delivered: 8, Read: 5, Failed: 2}
	repo.stored[id] = campaign.RecipientStatusCounts{Sent: 9, Delivered: 8, Read: 5, Failed: 2}

	metrics := &delivery.Metrics{}
	reconciler := delivery.NewReconciler(repo, metrics, nil, time.Hour)
	if err := reconciler.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.repaired) != 1 {
		t.Fatalf("expected repair, got %v", repo.repaired)
	}
	if repo.stored[id].Sent != 10 {
		t.Fatalf("expected repaired sent count")
	}
	if metrics.Snapshot()["reconciliation_repairs_total"] != 1 {
		t.Fatal("expected reconciliation metric")
	}
}
