package uow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type recordingCampaignRepo struct {
	createCalls int
	jobCalls    int
	snapCalls   int
	snapErr     error
}

func (r *recordingCampaignRepo) Create(context.Context, *campaign.Campaign, []campaign.Audience) error {
	r.createCalls++
	return nil
}
func (r *recordingCampaignRepo) CreateJob(context.Context, *campaign.BroadcastJob) error {
	r.jobCalls++
	return nil
}
func (r *recordingCampaignRepo) CreateAudienceSnapshot(context.Context, *campaign.AudienceSnapshot) error {
	r.snapCalls++
	return r.snapErr
}

func (r *recordingCampaignRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *recordingCampaignRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*campaign.Campaign, error) {
	return nil, repository.ErrNotFound
}
func (r *recordingCampaignRepo) List(context.Context, uuid.UUID, campaign.ListFilter, repository.PageParams) (repository.Page[campaign.Campaign], error) {
	return repository.Page[campaign.Campaign]{}, nil
}
func (r *recordingCampaignRepo) Update(context.Context, *campaign.Campaign) error { return nil }
func (r *recordingCampaignRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, campaign.Status) error {
	return nil
}
func (r *recordingCampaignRepo) ListAudiences(context.Context, uuid.UUID) ([]campaign.Audience, error) {
	return nil, nil
}
func (r *recordingCampaignRepo) UpdateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (r *recordingCampaignRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*campaign.BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (r *recordingCampaignRepo) CreateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *recordingCampaignRepo) UpdateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (r *recordingCampaignRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]campaign.BroadcastBatch, error) {
	return nil, nil
}
func (r *recordingCampaignRepo) BulkCreateRecipients(context.Context, []campaign.Recipient, time.Time) error {
	return nil
}
func (r *recordingCampaignRepo) UpdateRecipientStatus(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, *string, *string) error {
	return nil
}
func (r *recordingCampaignRepo) FindRecipientByWAMID(context.Context, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (r *recordingCampaignRepo) ApplyCounterDelta(context.Context, uuid.UUID, campaign.CounterDelta) error {
	return nil
}

type stubFactory struct {
	repo campaign.Repository
}

func (f stubFactory) Campaigns(database.Querier) campaign.Repository { return f.repo }

type noopManager struct {
	repo campaign.Repository
}

func (m *noopManager) Run(ctx context.Context, fn func(context.Context, UnitOfWork) error) error {
	work := &unitOfWork{factory: stubFactory{repo: m.repo}}
	return fn(ctx, work)
}

func TestCampaignRunner_PropagatesSnapshotError(t *testing.T) {
	repo := &recordingCampaignRepo{snapErr: errors.New("snapshot failed")}
	runner := NewCampaignRunnerWithManager(&noopManager{repo: repo})

	err := runner.Run(context.Background(), func(ctx context.Context, repos campaign.Repository) error {
		if err := repos.Create(ctx, &campaign.Campaign{}, nil); err != nil {
			return err
		}
		if err := repos.CreateJob(ctx, &campaign.BroadcastJob{}); err != nil {
			return err
		}
		return repos.CreateAudienceSnapshot(ctx, &campaign.AudienceSnapshot{})
	})
	if err == nil {
		t.Fatal("expected snapshot error")
	}
	if repo.createCalls != 1 || repo.jobCalls != 1 || repo.snapCalls != 1 {
		t.Fatalf("calls create=%d job=%d snap=%d", repo.createCalls, repo.jobCalls, repo.snapCalls)
	}
}
