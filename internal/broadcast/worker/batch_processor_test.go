package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

type mockMeta struct {
	mu      sync.Mutex
	calls   int
	wamid   string
	errs    []error
	delay   bool
}

func (m *mockMeta) SendTemplateMessage(context.Context, string, string, string, string, string, []metaclient.TemplateComponent) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if len(m.errs) > 0 {
		err := m.errs[0]
		m.errs = m.errs[1:]
		return "", err
	}
	if m.wamid == "" {
		m.wamid = "wamid.test.123"
	}
	return m.wamid, nil
}

type mockSendRepo struct {
	mu          sync.Mutex
	batch       campaign.BroadcastBatch
	recipients  []campaign.Recipient
	claimed     int
	completed   int
	requeued    int
	sentInc     int
	failedInc   int
	leaseRefresh int
}

func (m *mockSendRepo) FindBatchByID(context.Context, uuid.UUID) (*campaign.BroadcastBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.batch
	return &b, nil
}

func (m *mockSendRepo) ClaimBatchForProcessing(_ context.Context, _ uuid.UUID, _ string, _ time.Time) (*campaign.BroadcastBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.batch.Status == campaign.BatchCompleted {
		return &m.batch, nil
	}
	m.claimed++
	m.batch.Status = campaign.BatchProcessing
	now := time.Now().UTC()
	m.batch.LockedAt = &now
	return &m.batch, nil
}

func (m *mockSendRepo) RefreshBatchLease(context.Context, uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaseRefresh++
	return nil
}

func (m *mockSendRepo) CompleteBatch(context.Context, uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completed++
	m.batch.Status = campaign.BatchCompleted
	return nil
}

func (m *mockSendRepo) RequeueBatch(context.Context, uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requeued++
	m.batch.Status = campaign.BatchQueued
	return nil
}

func (m *mockSendRepo) MarkBatchFailed(context.Context, uuid.UUID, string) error { return nil }

func (m *mockSendRepo) ListRecipientsByBatch(context.Context, uuid.UUID) ([]campaign.Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]campaign.Recipient, len(m.recipients))
	copy(out, m.recipients)
	return out, nil
}

func (m *mockSendRepo) TransitionCampaignToSending(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (m *mockSendRepo) BatchHasActiveRecipients(context.Context, uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recipients {
		if r.Status == campaign.RecipientQueued || r.Status == campaign.RecipientSending {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockSendRepo) RecoverStaleSendingRecipients(_ context.Context, _ time.Time, _ int) (int, error) {
	return 0, nil
}

func (m *mockSendRepo) IncrementBatchRecipientSent(context.Context, uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentInc++
	return nil
}

func (m *mockSendRepo) IncrementBatchRecipientFailed(context.Context, uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedInc++
	return nil
}

type mockCampaignRepo struct {
	mu          sync.Mutex
	campaign    *campaign.Campaign
	sendRepo    *mockSendRepo
	updates     []string
	counterSent int
}

func (m *mockCampaignRepo) UpdateRecipientDelivery(_ context.Context, id uuid.UUID, from, to campaign.RecipientStatus, update campaign.RecipientDeliveryUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, string(from)+"->"+string(to))
	for i := range m.sendRepo.recipients {
		if m.sendRepo.recipients[i].ID == id {
			if update.IncrementAttempt {
				m.sendRepo.recipients[i].AttemptCount++
			}
			if update.WAMID != nil {
				m.sendRepo.recipients[i].WAMID = update.WAMID
			}
			if m.sendRepo.recipients[i].Status != to {
				m.sendRepo.recipients[i].Status = to
			}
			return nil
		}
	}
	return repository.ErrNotFound
}

func (m *mockCampaignRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*campaign.Campaign, error) {
	return m.campaign, nil
}

func (m *mockCampaignRepo) ApplyCounterDelta(context.Context, uuid.UUID, campaign.CounterDelta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counterSent++
	return nil
}

type mockTemplateRepo struct{ tpl *template.Template }

func (m *mockTemplateRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*template.Template, error) {
	return m.tpl, nil
}

type mockChannelRepo struct {
	ch    *channel.Channel
	token []byte
}

func (m *mockChannelRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	return m.ch, nil
}

func (m *mockChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return &channel.Credentials{AccessTokenEnc: m.token}, nil
}

type mockContactRepo struct{}

func (m *mockContactRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*contact.Contact, error) {
	return &contact.Contact{Name: "Test"}, nil
}

type mockFinalizer struct {
	campRepo *mockCampaignRepo
}

func (m *mockFinalizer) FinalizeRecipientSent(_ context.Context, in campaign.FinalizeSentInput) (bool, error) {
	if err := m.campRepo.UpdateRecipientDelivery(context.Background(), in.RecipientID, in.From, campaign.RecipientSent, campaign.RecipientDeliveryUpdate{
		WAMID:            &in.WAMID,
		ProviderResponse: in.ProviderResponse,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (m *mockFinalizer) FinalizeRecipientFailed(_ context.Context, in campaign.FinalizeFailedInput) (bool, error) {
	errMsg := in.LastError
	if err := m.campRepo.UpdateRecipientDelivery(context.Background(), in.RecipientID, in.From, campaign.RecipientFailed, campaign.RecipientDeliveryUpdate{
		LastError:   &errMsg,
		FailureCode: &in.FailureCode,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func newTestProcessor(t *testing.T, meta *mockMeta, send *mockSendRepo) (*BatchProcessor, *Metrics) {
	t.Helper()
	phone := "123456"
	ch := &channel.Channel{ID: uuid.New(), PhoneNumberID: &phone}
	camp := &campaign.Campaign{
		ID:              uuid.New(),
		TeamID:          uuid.New(),
		ChannelID:       ch.ID,
		TemplateID:      uuid.New(),
		VariableMapping: json.RawMessage(`{"body":{"1":"name"}}`),
	}
	campRepo := &mockCampaignRepo{campaign: camp, sendRepo: send}
	tpl := &template.Template{
		ID:       camp.TemplateID,
		Name:     "hello",
		Language: "en",
		Components: json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}"}]`),
	}
	cipher := testCipher(t)
	metrics := NewMetrics()
	sender := NewSender(meta, &mockChannelRepo{ch: ch, token: encryptToken(t, cipher)}, cipher, metrics, nil)
	cfg := DefaultConfig()
	cfg.WorkerID = "test-worker"
	cfg.RetryDelayFn = func(int) time.Duration { return time.Millisecond }
	cfg.RateLimitBackoff = time.Millisecond
	return NewBatchProcessor(cfg, send, campRepo, &mockTemplateRepo{tpl: tpl}, &mockChannelRepo{ch: ch, token: encryptToken(t, cipher)}, &mockContactRepo{}, sender, &mockFinalizer{campRepo: campRepo}, nil, NewMemoryRateLimiter(1000), metrics, nil), metrics
}

func testCipher(t *testing.T) *cryptoutil.TokenCipher {
	t.Helper()
	c, err := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func encryptToken(t *testing.T, c *cryptoutil.TokenCipher) []byte {
	t.Helper()
	raw, err := c.Encrypt("token")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSendOneRecipientStoresWAMID(t *testing.T) {
	meta := &mockMeta{wamid: "wamid.real.1"}
	batchID := uuid.New()
	recID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchQueued, JobID: uuid.New(), CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: recID, Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "15551234567",
		}},
	}
	p, _ := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		JobID: batchID.String(), BatchID: batchID.String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(), RecipientCount: 1,
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if send.completed != 1 {
		t.Fatalf("expected batch completed, got %d", send.completed)
	}
	if meta.calls != 1 {
		t.Fatalf("expected 1 meta call, got %d", meta.calls)
	}
	if send.recipients[0].WAMID == nil || *send.recipients[0].WAMID != "wamid.real.1" {
		t.Fatalf("expected wamid stored")
	}
	if send.recipients[0].Status != campaign.RecipientSent {
		t.Fatalf("expected sent status, got %s", send.recipients[0].Status)
	}
}

func TestRetry500SucceedsSecondAttempt(t *testing.T) {
	meta := &mockMeta{
		wamid: "wamid.retry",
		errs:  []error{metaclient.NewAPIError(500, "internal error")},
	}
	batchID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchQueued, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: uuid.New(), Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "15551234567",
		}},
	}
	p, _ := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(),
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if meta.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", meta.calls)
	}
	if send.recipients[0].Status != campaign.RecipientSent {
		t.Fatalf("expected sent, got %s", send.recipients[0].Status)
	}
}

func TestPermanentFailureInvalidPhone(t *testing.T) {
	meta := &mockMeta{errs: []error{metaclient.NewAPIError(400, `{"error":{"message":"invalid phone number","code":131026}}`)}}
	batchID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchQueued, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: uuid.New(), Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "bad",
		}},
	}
	p, _ := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(),
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if send.recipients[0].Status != campaign.RecipientFailed {
		t.Fatalf("expected failed, got %s", send.recipients[0].Status)
	}
	if send.failedInc != 1 {
		t.Fatalf("expected batch failed increment")
	}
}

func TestIdempotencyDuplicateBatchEvent(t *testing.T) {
	meta := &mockMeta{wamid: "wamid.once"}
	batchID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchCompleted, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: uuid.New(), Status: campaign.RecipientSent, ContactID: uuid.New(), Phone: "15551234567",
			WAMID: strPtr("wamid.once"),
		}},
	}
	p, _ := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(),
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if meta.calls != 0 {
		t.Fatalf("expected no meta calls on completed batch, got %d", meta.calls)
	}
}

func TestRateLimit429DelaysAndRetries(t *testing.T) {
	meta := &mockMeta{
		wamid: "wamid.rl",
		errs:  []error{metaclient.NewAPIError(429, "rate limit"), nil},
	}
	batchID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchQueued, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: uuid.New(), Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "15551234567",
		}},
	}
	p, metrics := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(),
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if metrics.MetaRateLimit < 1 {
		t.Fatalf("expected rate limit metric")
	}
	if send.recipients[0].Status != campaign.RecipientSent {
		t.Fatalf("expected sent after 429 retry")
	}
}

func TestWorkerRestartBatchContinues(t *testing.T) {
	meta := &mockMeta{wamid: "wamid.cont"}
	batchID := uuid.New()
	wamid := "wamid.done"
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchProcessing, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{
			{ID: uuid.New(), Status: campaign.RecipientSent, ContactID: uuid.New(), Phone: "1", WAMID: &wamid},
			{ID: uuid.New(), Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "2"},
		},
	}
	p, _ := newTestProcessor(t, meta, send)
	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(), RecipientCount: 2,
	}
	if err := p.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if meta.calls != 1 {
		t.Fatalf("expected only one send for remaining recipient, got %d", meta.calls)
	}
	if send.completed != 1 {
		t.Fatalf("expected batch completion")
	}
}

func strPtr(s string) *string { return &s }

// Ensure mockMeta implements MetaSender.
var _ MetaSender = (*mockMeta)(nil)

// Ensure mocks satisfy interfaces used in tests.
var _ campaign.SendRepository = (*mockSendRepo)(nil)

func TestValidationFailureNoMetaCall(t *testing.T) {
	meta := &mockMeta{wamid: "wamid.never"}
	batchID := uuid.New()
	send := &mockSendRepo{
		batch: campaign.BroadcastBatch{ID: batchID, Status: campaign.BatchQueued, CampaignID: uuid.New()},
		recipients: []campaign.Recipient{{
			ID: uuid.New(), Status: campaign.RecipientQueued, ContactID: uuid.New(), Phone: "15551234567",
		}},
	}

	phone := "123456"
	ch := &channel.Channel{ID: uuid.New(), PhoneNumberID: &phone}
	camp := &campaign.Campaign{
		ID:              uuid.New(),
		TeamID:          uuid.New(),
		ChannelID:       ch.ID,
		TemplateID:      uuid.New(),
		VariableMapping: json.RawMessage(`{"body":{"1":"name"}}`),
	}
	campRepo := &mockCampaignRepo{campaign: camp, sendRepo: send}
	tpl := &template.Template{
		ID: uuid.New(), Name: "missing", Language: "en",
		Components: json.RawMessage(`[{"type":"BODY","text":"Hello {{1}} {{2}}"}]`),
	}
	cipher := testCipher(t)
	metrics := NewMetrics()
	sender := NewSender(meta, &mockChannelRepo{ch: ch, token: encryptToken(t, cipher)}, cipher, metrics, nil)
	cfg := DefaultConfig()
	cfg.RetryDelayFn = func(int) time.Duration { return time.Millisecond }
	processor := NewBatchProcessor(cfg, send, campRepo, &mockTemplateRepo{tpl: tpl}, &mockChannelRepo{ch: ch, token: encryptToken(t, cipher)}, &mockContactRepo{}, sender, &mockFinalizer{campRepo: campRepo}, nil, NewMemoryRateLimiter(1000), metrics, nil)

	payload := sharedevents.BatchSendPayload{
		BatchID: batchID.String(), JobID: uuid.New().String(), CampaignID: send.batch.CampaignID.String(),
		TeamID: uuid.New().String(), ChannelID: uuid.New().String(),
	}
	if err := processor.HandleBatchSend(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if meta.calls != 0 {
		t.Fatalf("expected no meta calls, got %d", meta.calls)
	}
	if send.recipients[0].Status != campaign.RecipientFailed {
		t.Fatalf("expected failed recipient, got %s", send.recipients[0].Status)
	}
}

func TestClassifyValidationErrorPermanent(t *testing.T) {
	kind, code, _ := ClassifySendError(&campaign.SendValidationError{
		Code: campaign.ValidationCodeMissingVariable, Message: "body: missing mapping",
	})
	if kind != FailurePermanent || code != campaign.ValidationCodeMissingVariable {
		t.Fatalf("got kind=%v code=%s", kind, code)
	}
}

func TestClassifyPermanentTemplateError(t *testing.T) {
	kind, code, _ := ClassifySendError(metaclient.NewAPIError(400, "template paused"))
	if kind != FailurePermanent || code != "template_error" {
		t.Fatalf("got kind=%v code=%s", kind, code)
	}
}

func TestClassifyTransientTimeout(t *testing.T) {
	kind, _, _ := ClassifySendError(errors.New("connection timeout"))
	if kind != FailureTransient {
		t.Fatalf("expected transient")
	}
}
