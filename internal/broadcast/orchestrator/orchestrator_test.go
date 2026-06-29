package orchestrator_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/broadcast/orchestrator"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

type memStore struct {
	mu sync.Mutex

	campaigns       map[uuid.UUID]*campaign.Campaign
	jobs            map[uuid.UUID]*campaign.BroadcastJob
	batches         map[uuid.UUID]*campaign.BroadcastBatch
	recipients      []campaign.Recipient
	snapshots       int
	outbox          []campaign.OutboxMessage
	queued          map[uuid.UUID]int
	audiences       []campaign.Audience
}

func newMemStore() *memStore {
	return &memStore{
		campaigns: map[uuid.UUID]*campaign.Campaign{},
		jobs:      map[uuid.UUID]*campaign.BroadcastJob{},
		batches:   map[uuid.UUID]*campaign.BroadcastBatch{},
		queued:    map[uuid.UUID]int{},
	}
}

func (m *memStore) ClaimDueScheduled(_ context.Context, now time.Time, limit int) ([]campaign.Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []campaign.Campaign
	for _, c := range m.campaigns {
		if c.Status == campaign.StatusScheduled && c.ScheduledAt != nil && !c.ScheduledAt.After(now) {
			out = append(out, *c)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memStore) TransitionCampaignToQueued(_ context.Context, _, campaignID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.campaigns[campaignID]
	if c == nil || c.Status != campaign.StatusScheduled {
		return repository.ErrNotFound
	}
	c.Status = campaign.StatusQueued
	m.queued[campaignID]++
	return nil
}

func (m *memStore) FindActiveJobByCampaign(_ context.Context, campaignID uuid.UUID) (*campaign.BroadcastJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.CampaignID == campaignID && j.Status != campaign.JobCompleted && j.Status != campaign.JobFailed && j.Status != campaign.JobCancelled {
			cp := *j
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memStore) CreateJob(_ context.Context, job *campaign.BroadcastJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

func (m *memStore) InsertOutbox(_ context.Context, subject string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outbox = append(m.outbox, campaign.OutboxMessage{ID: uuid.New(), Subject: subject, Payload: payload})
	return nil
}

func (m *memStore) ListPendingOutbox(context.Context, int) ([]campaign.OutboxMessage, error) {
	return nil, nil
}
func (m *memStore) MarkOutboxPublished(context.Context, uuid.UUID) error { return nil }
func (m *memStore) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (m *memStore) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func (m *memStore) FindJobByID(_ context.Context, jobID uuid.UUID) (*campaign.BroadcastJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[jobID]
	if j == nil {
		return nil, repository.ErrNotFound
	}
	cp := *j
	return &cp, nil
}

func (m *memStore) UpdateJob(_ context.Context, job *campaign.BroadcastJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

func (m *memStore) CountRecipientsByJob(_ context.Context, jobID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.recipients {
		if r.JobID != nil && *r.JobID == jobID {
			n++
		}
	}
	return n, nil
}

func (m *memStore) FindByID(_ context.Context, _, id uuid.UUID) (*campaign.Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.campaigns[id]
	if c == nil {
		return nil, repository.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (m *memStore) ListAudiences(context.Context, uuid.UUID) ([]campaign.Audience, error) {
	return m.audiences, nil
}

func (m *memStore) LoadListEnrichment(context.Context, uuid.UUID, []campaign.Campaign) (map[uuid.UUID]campaign.ListEnrichment, error) {
	return map[uuid.UUID]campaign.ListEnrichment{}, nil
}

func (m *memStore) CreateAudienceSnapshot(context.Context, *campaign.AudienceSnapshot) error {
	m.snapshots++
	return nil
}

func (m *memStore) BulkCreateRecipients(_ context.Context, recs []campaign.Recipient, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recipients = append(m.recipients, recs...)
	return nil
}

func (m *memStore) UpdateCampaignTotalContacts(context.Context, uuid.UUID, int) error { return nil }

func (m *memStore) CountBatchesByJob(_ context.Context, jobID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, b := range m.batches {
		if b.JobID == jobID {
			n++
		}
	}
	return n, nil
}

func (m *memStore) PlanBatchesForJob(_ context.Context, jobID uuid.UUID, batchSize int) ([]campaign.BroadcastBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if batchSize <= 0 {
		batchSize = 100
	}
	for _, b := range m.batches {
		if b.JobID == jobID {
			return m.listBatchesLocked(jobID), nil
		}
	}
	var pending []uuid.UUID
	for _, r := range m.recipients {
		if r.JobID != nil && *r.JobID == jobID && r.Status == campaign.RecipientPending {
			pending = append(pending, r.ID)
		}
	}
	job := m.jobs[jobID]
	var created []campaign.BroadcastBatch
	num := 0
	for i := 0; i < len(pending); i += batchSize {
		num++
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[i:end]
		b := campaign.BroadcastBatch{
			ID:             uuid.New(),
			JobID:          jobID,
			CampaignID:     job.CampaignID,
			BatchNumber:    num,
			Status:         campaign.BatchPending,
			RecipientCount: len(chunk),
		}
		m.batches[b.ID] = &b
		created = append(created, b)
		for _, rid := range chunk {
			for idx := range m.recipients {
				if m.recipients[idx].ID == rid {
					m.recipients[idx].Status = campaign.RecipientQueued
					bid := b.ID
					m.recipients[idx].BatchID = &bid
				}
			}
		}
	}
	return created, nil
}

func (m *memStore) listBatchesLocked(jobID uuid.UUID) []campaign.BroadcastBatch {
	var out []campaign.BroadcastBatch
	for _, b := range m.batches {
		if b.JobID == jobID {
			out = append(out, *b)
		}
	}
	return out
}

func (m *memStore) ListBatchesByJob(_ context.Context, jobID uuid.UUID) ([]campaign.BroadcastBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listBatchesLocked(jobID), nil
}

func (m *memStore) MarkBatchesQueued(_ context.Context, ids []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if b := m.batches[id]; b != nil {
			b.Status = campaign.BatchQueued
		}
	}
	return nil
}

func (m *memStore) ListStaleProcessingBatches(_ context.Context, before time.Time, _ int) ([]campaign.BroadcastBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []campaign.BroadcastBatch
	for _, b := range m.batches {
		if b.Status == campaign.BatchProcessing && b.LockedAt != nil && b.LockedAt.Before(before) {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (m *memStore) RequeueStaleBatch(_ context.Context, batchID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.batches[batchID]
	if b == nil || b.Status != campaign.BatchProcessing {
		return repository.ErrNotFound
	}
	b.Status = campaign.BatchQueued
	b.LockedBy = nil
	b.LockedAt = nil
	return nil
}

func (m *memStore) ResetSendingRecipientsWithoutWAMID(context.Context, uuid.UUID) error { return nil }
func (m *memStore) ListPendingJobs(context.Context, int) ([]campaign.BroadcastJob, error) {
	return nil, nil
}

func (m *memStore) Create(context.Context, *campaign.Campaign, []campaign.Audience) error { return nil }
func (m *memStore) List(context.Context, uuid.UUID, campaign.ListFilter, repository.PageParams) (repository.Page[campaign.Campaign], error) {
	return repository.Page[campaign.Campaign]{}, nil
}
func (m *memStore) Update(context.Context, *campaign.Campaign) error                     { return nil }
func (m *memStore) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, campaign.Status) error {
	return nil
}
func (m *memStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memStore) CreateBatch(context.Context, *campaign.BroadcastBatch) error { return nil }
func (m *memStore) UpdateBatch(context.Context, *campaign.BroadcastBatch) error  { return nil }
func (m *memStore) UpdateRecipientStatus(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, *string, *string) error {
	return nil
}
func (m *memStore) UpdateRecipientDelivery(context.Context, uuid.UUID, campaign.RecipientStatus, campaign.RecipientStatus, campaign.RecipientDeliveryUpdate) error {
	return nil
}
func (m *memStore) FindRecipientByWAMID(context.Context, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (m *memStore) FindRecipientByWAMIDForTeam(context.Context, uuid.UUID, string) (*campaign.Recipient, error) {
	return nil, repository.ErrNotFound
}
func (m *memStore) ApplyCounterDelta(context.Context, uuid.UUID, campaign.CounterDelta) error { return nil }
func (m *memStore) CountNonTerminalRecipients(context.Context, uuid.UUID) (int, error)        { return 0, nil }
func (m *memStore) CompleteCampaign(context.Context, uuid.UUID, uuid.UUID, time.Time) error { return nil }
func (m *memStore) AggregateRecipientStatusCounts(context.Context, uuid.UUID) (campaign.RecipientStatusCounts, error) {
	return campaign.RecipientStatusCounts{}, nil
}
func (m *memStore) SetCampaignCounters(context.Context, uuid.UUID, campaign.RecipientStatusCounts) error {
	return nil
}
func (m *memStore) RepairCampaignCounters(context.Context, uuid.UUID) (bool, error) { return false, nil }
func (m *memStore) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}

type noopPublisher struct{}

func (noopPublisher) PublishJSON(context.Context, string, any) error { return nil }

type memContacts struct {
	byID map[uuid.UUID]contact.Contact
}

func (m *memContacts) FindByID(_ context.Context, _ uuid.UUID, id uuid.UUID) (*contact.Contact, error) {
	c, ok := m.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &c, nil
}

func (m *memContacts) ListByIDs(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]contact.Contact, error) {
	var out []contact.Contact
	for _, id := range ids {
		if c, ok := m.byID[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memContacts) Create(context.Context, *contact.Contact) error { return nil }
func (m *memContacts) FindByPhone(context.Context, uuid.UUID, string) (*contact.Contact, error) {
	return nil, repository.ErrNotFound
}
func (m *memContacts) List(context.Context, uuid.UUID, contact.ListFilter, repository.PageParams) (repository.Page[contact.Contact], error) {
	return repository.Page[contact.Contact]{}, nil
}
func (m *memContacts) Update(context.Context, *contact.Contact) error { return nil }
func (m *memContacts) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memContacts) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }

type memSegments struct{}

func (memSegments) ListMemberContactIDs(context.Context, uuid.UUID, repository.PageParams) ([]uuid.UUID, int64, error) {
	return nil, 0, nil
}

type memTags struct{}

func (memTags) ListContactIDsByTag(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

func TestSchedulerPicksOnce(t *testing.T) {
	store := newMemStore()
	cid := uuid.New()
	tid := uuid.New()
	ch := uuid.New()
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	store.campaigns[cid] = &campaign.Campaign{ID: cid, TeamID: tid, ChannelID: ch, Status: campaign.StatusScheduled, ScheduledAt: &past}
	jid := uuid.New()
	store.jobs[jid] = &campaign.BroadcastJob{ID: jid, CampaignID: cid, TeamID: tid, ChannelID: ch, Status: campaign.JobPending}

	sched := orchestrator.NewScheduler(store, store, &orchestrator.Metrics{}, nil)
	if err := sched.Tick(context.Background(), now, 10); err != nil {
		t.Fatal(err)
	}
	if store.queued[cid] != 1 {
		t.Fatalf("expected queued once, got %d", store.queued[cid])
	}
	if err := sched.Tick(context.Background(), now, 10); err != nil {
		t.Fatal(err)
	}
	if store.queued[cid] != 1 {
		t.Fatalf("duplicate queue on second tick: %d", store.queued[cid])
	}
	if len(store.outbox) != 1 {
		t.Fatalf("expected one outbox message, got %d", len(store.outbox))
	}
}

func TestAudienceResolverDeduplicates(t *testing.T) {
	store := newMemStore()
	jobID := uuid.New()
	cid := uuid.New()
	tid := uuid.New()
	ch := uuid.New()
	store.jobs[jobID] = &campaign.BroadcastJob{ID: jobID, CampaignID: cid, TeamID: tid, ChannelID: ch, Status: campaign.JobPending}
	store.campaigns[cid] = &campaign.Campaign{ID: cid, TeamID: tid, ChannelID: ch}
	contactID := uuid.New()
	store.audiences = []campaign.Audience{
		{SourceType: campaign.AudienceContact, SourceID: contactID},
		{SourceType: campaign.AudienceContact, SourceID: contactID},
	}
	contacts := &memContacts{byID: map[uuid.UUID]contact.Contact{
		contactID: {ID: contactID, TeamID: tid, Phone: "+628123456789", MarketingStatus: contact.MarketingSubscribed},
	}}

	resolver := orchestrator.NewAudienceResolver(store, contacts, memSegments{}, memTags{}, &orchestrator.Metrics{}, nil, 1000)
	if err := resolver.Resolve(context.Background(), store.jobs[jobID]); err != nil {
		t.Fatal(err)
	}
	if len(store.recipients) != 1 {
		t.Fatalf("expected 1 recipient after dedupe, got %d", len(store.recipients))
	}
	if store.snapshots != 1 {
		t.Fatal("expected snapshot")
	}
}

func TestBatchPlannerChunking(t *testing.T) {
	runPlan := func(pending int) int {
		store := newMemStore()
		jobID := uuid.New()
		cid := uuid.New()
		tid := uuid.New()
		ch := uuid.New()
		store.jobs[jobID] = &campaign.BroadcastJob{ID: jobID, CampaignID: cid, TeamID: tid, ChannelID: ch, Status: campaign.JobPlanningBatches}
		for i := 0; i < pending; i++ {
			jid := jobID
			store.recipients = append(store.recipients, campaign.Recipient{
				ID: uuid.New(), CampaignID: cid, TeamID: tid, JobID: &jid, Status: campaign.RecipientPending,
			})
		}
		planner := orchestrator.NewBatchPlanner(store, noopPublisher{}, store, &orchestrator.Metrics{}, 100, nil)
		if err := planner.Plan(context.Background(), store.jobs[jobID]); err != nil {
			t.Fatalf("plan: %v", err)
		}
		return len(store.batches)
	}
	if n := runPlan(100); n != 1 {
		t.Fatalf("100 recipients => 1 batch, got %d", n)
	}
	if n := runPlan(250); n != 3 {
		t.Fatalf("250 recipients => 3 batches, got %d", n)
	}
}

func TestLeaseRecovery(t *testing.T) {
	store := newMemStore()
	jobID := uuid.New()
	batchID := uuid.New()
	cid := uuid.New()
	tid := uuid.New()
	ch := uuid.New()
	locked := time.Now().UTC().Add(-20 * time.Minute)
	worker := "worker-1"
	store.jobs[jobID] = &campaign.BroadcastJob{ID: jobID, CampaignID: cid, TeamID: tid, ChannelID: ch, Status: campaign.JobSending}
	store.batches[batchID] = &campaign.BroadcastBatch{
		ID: batchID, JobID: jobID, CampaignID: cid, Status: campaign.BatchProcessing,
		LockedAt: &locked, LockedBy: &worker, RecipientCount: 10,
	}
	freshID := uuid.New()
	freshLock := time.Now().UTC()
	store.batches[freshID] = &campaign.BroadcastBatch{
		ID: freshID, JobID: jobID, CampaignID: cid, Status: campaign.BatchProcessing,
		LockedAt: &freshLock, LockedBy: &worker,
	}

	rec := orchestrator.NewRecovery(store, noopPublisher{}, store, &orchestrator.Metrics{}, 10*time.Minute, nil)
	if err := rec.RunOnce(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if store.batches[batchID].Status != campaign.BatchQueued {
		t.Fatal("expired batch should be requeued")
	}
	if store.batches[freshID].Status != campaign.BatchProcessing {
		t.Fatal("fresh batch should remain processing")
	}
}

func TestOutboxJobCreatedPayload(t *testing.T) {
	store := newMemStore()
	job := &campaign.BroadcastJob{
		ID: uuid.New(), CampaignID: uuid.New(), TeamID: uuid.New(), ChannelID: uuid.New(),
	}
	if err := orchestrator.EnqueueJobCreated(context.Background(), store, job); err != nil {
		t.Fatal(err)
	}
	var payload sharedevents.JobCreatedPayload
	if err := json.Unmarshal(store.outbox[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.JobID != job.ID.String() {
		t.Fatal("job id mismatch")
	}
}

func TestConcurrentSchedulerTick(t *testing.T) {
	store := newMemStore()
	cid := uuid.New()
	tid := uuid.New()
	ch := uuid.New()
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	store.campaigns[cid] = &campaign.Campaign{ID: cid, TeamID: tid, ChannelID: ch, Status: campaign.StatusScheduled, ScheduledAt: &past}
	jid := uuid.New()
	store.jobs[jid] = &campaign.BroadcastJob{ID: jid, CampaignID: cid, TeamID: tid, ChannelID: ch, Status: campaign.JobPending}

	sched := orchestrator.NewScheduler(store, store, &orchestrator.Metrics{}, nil)
	const workers = 5
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			errCh <- sched.Tick(context.Background(), now, 10)
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if store.queued[cid] != 1 {
		t.Fatalf("expected exactly one queue transition, got %d", store.queued[cid])
	}
	if len(store.outbox) != 1 {
		t.Fatalf("expected one outbox message, got %d", len(store.outbox))
	}
}
