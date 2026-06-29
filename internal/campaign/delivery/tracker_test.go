package delivery_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	"github.com/omnichannel/omni-chan-v2/internal/report"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

type memCampaignRepo struct {
	recipients map[string]*campaign.Recipient
	counters   map[uuid.UUID]campaign.CounterDelta
	completed  map[uuid.UUID]bool
	nonTerm    map[uuid.UUID]int
	updates    []statusUpdate
}

type statusUpdate struct {
	id   uuid.UUID
	from campaign.RecipientStatus
	to   campaign.RecipientStatus
}

func newMemCampaignRepo() *memCampaignRepo {
	return &memCampaignRepo{
		recipients: map[string]*campaign.Recipient{},
		counters:   map[uuid.UUID]campaign.CounterDelta{},
		completed:  map[uuid.UUID]bool{},
		nonTerm:    map[uuid.UUID]int{},
	}
}

func (m *memCampaignRepo) key(teamID uuid.UUID, wamid string) string {
	return teamID.String() + ":" + wamid
}

func (m *memCampaignRepo) seed(rec *campaign.Recipient) {
	m.recipients[m.key(rec.TeamID, *rec.WAMID)] = rec
}

func (m *memCampaignRepo) Create(context.Context, *campaign.Campaign, []campaign.Audience) error {
	return nil
}
func (m *memCampaignRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*campaign.Campaign, error) {
	return nil, repository.ErrNotFound
}
func (m *memCampaignRepo) List(context.Context, uuid.UUID, campaign.ListFilter, repository.PageParams) (repository.Page[campaign.Campaign], error) {
	return repository.Page[campaign.Campaign]{}, nil
}
func (m *memCampaignRepo) Update(context.Context, *campaign.Campaign) error { return nil }
func (m *memCampaignRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, campaign.Status) error {
	return nil
}
func (m *memCampaignRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memCampaignRepo) ListAudiences(context.Context, uuid.UUID) ([]campaign.Audience, error) {
	return nil, nil
}
func (m *memCampaignRepo) LoadListEnrichment(context.Context, uuid.UUID, []campaign.Campaign) (map[uuid.UUID]campaign.ListEnrichment, error) {
	return map[uuid.UUID]campaign.ListEnrichment{}, nil
}
func (m *memCampaignRepo) CreateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (m *memCampaignRepo) UpdateJob(context.Context, *campaign.BroadcastJob) error { return nil }
func (m *memCampaignRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*campaign.BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (m *memCampaignRepo) CreateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (m *memCampaignRepo) UpdateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (m *memCampaignRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]campaign.BroadcastBatch, error) {
	return nil, nil
}
func (m *memCampaignRepo) CreateAudienceSnapshot(context.Context, *campaign.AudienceSnapshot) error {
	return nil
}
func (m *memCampaignRepo) BulkCreateRecipients(context.Context, []campaign.Recipient, time.Time) error {
	return nil
}
func (m *memCampaignRepo) UpdateRecipientStatus(ctx context.Context, id uuid.UUID, from, to campaign.RecipientStatus, wamid *string, lastError *string) error {
	return m.UpdateRecipientDelivery(ctx, id, from, to, campaign.RecipientDeliveryUpdate{WAMID: wamid, LastError: lastError})
}
func (m *memCampaignRepo) UpdateRecipientDelivery(_ context.Context, id uuid.UUID, from, to campaign.RecipientStatus, _ campaign.RecipientDeliveryUpdate) error {
	for _, rec := range m.recipients {
		if rec.ID == id {
			if rec.Status != from {
				return repository.ErrNotFound
			}
			rec.Status = to
			m.updates = append(m.updates, statusUpdate{id: id, from: from, to: to})
			return nil
		}
	}
	return repository.ErrNotFound
}
func (m *memCampaignRepo) FindRecipientByWAMID(_ context.Context, wamid string) (*campaign.Recipient, error) {
	for _, rec := range m.recipients {
		if rec.WAMID != nil && *rec.WAMID == wamid {
			return rec, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *memCampaignRepo) FindRecipientByWAMIDForTeam(_ context.Context, teamID uuid.UUID, wamid string) (*campaign.Recipient, error) {
	rec, ok := m.recipients[m.key(teamID, wamid)]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return rec, nil
}
func (m *memCampaignRepo) ApplyCounterDelta(_ context.Context, campaignID uuid.UUID, delta campaign.CounterDelta) error {
	cur := m.counters[campaignID]
	cur.Sent += delta.Sent
	cur.Delivered += delta.Delivered
	cur.Read += delta.Read
	cur.Failed += delta.Failed
	m.counters[campaignID] = cur
	return nil
}
func (m *memCampaignRepo) CountNonTerminalRecipients(_ context.Context, campaignID uuid.UUID) (int, error) {
	return m.nonTerm[campaignID], nil
}
func (m *memCampaignRepo) CompleteCampaign(_ context.Context, teamID, campaignID uuid.UUID, _ time.Time) error {
	m.completed[campaignID] = true
	return nil
}
func (m *memCampaignRepo) AggregateRecipientStatusCounts(context.Context, uuid.UUID) (campaign.RecipientStatusCounts, error) {
	return campaign.RecipientStatusCounts{}, nil
}
func (m *memCampaignRepo) SetCampaignCounters(context.Context, uuid.UUID, campaign.RecipientStatusCounts) error {
	return nil
}
func (m *memCampaignRepo) RepairCampaignCounters(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (m *memCampaignRepo) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *memCampaignRepo) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (m *memCampaignRepo) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

type memReportRepo struct {
	deltas []report.DailyStatDelta
}

func (m *memReportRepo) AggregateDashboard(context.Context, uuid.UUID, report.DateRange, *uuid.UUID) (*report.DashboardStats, error) {
	return &report.DashboardStats{}, nil
}
func (m *memReportRepo) ListDailyStats(context.Context, uuid.UUID, report.DateRange) ([]report.DailyStat, error) {
	return nil, nil
}
func (m *memReportRepo) IncrementDailyStat(_ context.Context, _ uuid.UUID, _ time.Time, delta report.DailyStatDelta) error {
	m.deltas = append(m.deltas, delta)
	return nil
}
func (m *memReportRepo) GetCampaignFunnel(context.Context, uuid.UUID, uuid.UUID) (*report.CampaignFunnel, error) {
	return nil, repository.ErrNotFound
}
func (m *memReportRepo) ListCampaignFunnels(context.Context, uuid.UUID, report.DateRange, repository.PageParams) (repository.Page[report.CampaignFunnel], error) {
	return repository.Page[report.CampaignFunnel]{}, nil
}
func (m *memReportRepo) RecordLinkClick(context.Context, *report.LinkClick) error { return nil }
func (m *memReportRepo) ListLinkClicks(context.Context, uuid.UUID, uuid.UUID, repository.PageParams) (repository.Page[report.LinkClick], error) {
	return repository.Page[report.LinkClick]{}, nil
}

func newTracker(repo *memCampaignRepo, reports *memReportRepo) *delivery.Tracker {
	metrics := &delivery.Metrics{}
	agg := delivery.NewReportAggregator(repo, reports)
	completion := delivery.NewCompletionMonitor(repo, metrics, nil)
	return delivery.NewTracker(repo, agg, completion, metrics, nil)
}

func TestTrackerSentDeliveredReadLifecycle(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	wamid := "wamid-1"
	rec := &campaign.Recipient{
		ID:         uuid.New(),
		CampaignID: campaignID,
		TeamID:     teamID,
		Status:     campaign.RecipientSending,
		WAMID:      &wamid,
	}
	repo := newMemCampaignRepo()
	repo.seed(rec)
	reports := &memReportRepo{}
	tracker := newTracker(repo, reports)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, eventType := range []string{
		sharedevents.TypeMessageStatusSent,
		sharedevents.TypeMessageStatusDelivered,
		sharedevents.TypeMessageStatusRead,
	} {
		if err := tracker.HandleStatus(ctx, teamID, eventType, wamid, "", nil, now); err != nil {
			t.Fatalf("handle %s: %v", eventType, err)
		}
	}
	if rec.Status != campaign.RecipientRead {
		t.Fatalf("expected read, got %s", rec.Status)
	}
	c := repo.counters[campaignID]
	if c.Sent != 1 || c.Delivered != 1 || c.Read != 1 {
		t.Fatalf("unexpected counters: %+v", c)
	}
	if len(reports.deltas) != 3 {
		t.Fatalf("expected 3 daily stat deltas, got %d", len(reports.deltas))
	}
}

func TestTrackerDuplicateWebhookIgnored(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	wamid := "wamid-dup"
	rec := &campaign.Recipient{
		ID:         uuid.New(),
		CampaignID: campaignID,
		TeamID:     teamID,
		Status:     campaign.RecipientDelivered,
		WAMID:      &wamid,
	}
	repo := newMemCampaignRepo()
	repo.seed(rec)
	reports := &memReportRepo{}
	tracker := newTracker(repo, reports)

	if err := tracker.HandleStatus(context.Background(), teamID, sharedevents.TypeMessageStatusDelivered, wamid, "", nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("duplicate should not update recipient")
	}
	if len(reports.deltas) != 0 {
		t.Fatalf("duplicate should not increment stats")
	}
}

func TestTrackerInvalidRegressionIgnored(t *testing.T) {
	teamID := uuid.New()
	wamid := "wamid-regress"
	rec := &campaign.Recipient{
		ID:     uuid.New(),
		TeamID: teamID,
		Status: campaign.RecipientRead,
		WAMID:  &wamid,
	}
	repo := newMemCampaignRepo()
	repo.seed(rec)
	tracker := newTracker(repo, &memReportRepo{})

	if err := tracker.HandleStatus(context.Background(), teamID, sharedevents.TypeMessageStatusDelivered, wamid, "", nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(repo.updates) != 0 {
		t.Fatal("read -> delivered must be ignored")
	}
}

func TestTrackerFailedStatus(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	wamid := "wamid-fail"
	rec := &campaign.Recipient{
		ID:         uuid.New(),
		CampaignID: campaignID,
		TeamID:     teamID,
		Status:     campaign.RecipientSent,
		WAMID:      &wamid,
	}
	repo := newMemCampaignRepo()
	repo.seed(rec)
	reports := &memReportRepo{}
	tracker := newTracker(repo, reports)

	errText := "Message failed: invalid recipient"
	if err := tracker.HandleStatus(context.Background(), teamID, sharedevents.TypeMessageStatusFailed, wamid, errText, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if rec.Status != campaign.RecipientFailed {
		t.Fatalf("expected failed, got %s", rec.Status)
	}
	if repo.counters[campaignID].Failed != 1 {
		t.Fatalf("expected failed counter increment")
	}
}

func TestTrackerCrossTeamIsolation(t *testing.T) {
	teamA := uuid.New()
	teamB := uuid.New()
	wamid := "wamid-iso"
	rec := &campaign.Recipient{
		ID:     uuid.New(),
		TeamID: teamA,
		Status: campaign.RecipientSending,
		WAMID:  &wamid,
	}
	repo := newMemCampaignRepo()
	repo.seed(rec)
	tracker := newTracker(repo, &memReportRepo{})

	if err := tracker.HandleStatus(context.Background(), teamB, sharedevents.TypeMessageStatusSent, wamid, "", nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if rec.Status != campaign.RecipientSending {
		t.Fatal("other team must not update recipient")
	}
}

func TestCompletionMonitor(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	repo := newMemCampaignRepo()
	repo.nonTerm[campaignID] = 0
	monitor := delivery.NewCompletionMonitor(repo, &delivery.Metrics{}, nil)

	done, err := monitor.Check(context.Background(), teamID, campaignID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected completion")
	}
	if !repo.completed[campaignID] {
		t.Fatal("campaign should be marked completed")
	}
}

func TestCounterDeltaIdempotency(t *testing.T) {
	from := campaign.RecipientSent
	to := campaign.RecipientDelivered
	d1 := delivery.CounterDeltaForTransition(from, to)
	d2 := delivery.CounterDeltaForTransition(to, to)
	if d1.Delivered != 1 || d2.Delivered != 0 {
		t.Fatalf("counter delta must be idempotent for same status")
	}
}

func TestCanTransitionMonotonic(t *testing.T) {
	if delivery.CanTransition(campaign.RecipientRead, campaign.RecipientDelivered) {
		t.Fatal("read -> delivered must be invalid")
	}
	if !delivery.CanTransition(campaign.RecipientSent, campaign.RecipientDelivered) {
		t.Fatal("sent -> delivered must be valid")
	}
}

func TestClassifyFailure(t *testing.T) {
	code, _ := delivery.ClassifyFailure("Error 131026 invalid recipient number")
	if code != "invalid_number" {
		t.Fatalf("expected invalid_number, got %s", code)
	}
}
