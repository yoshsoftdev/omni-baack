package inbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

// ledgerStore mimics inbox_window_recontact_sends with its
// UNIQUE (conversation_id, customer_message_at) constraint.
type ledgerStore struct {
	WindowRecontactStore // unused methods panic if called

	mu      sync.Mutex
	claims  map[string]uuid.UUID // key → send id
	sent    map[uuid.UUID]uuid.UUID
	ownerID uuid.UUID
}

func newLedgerStore() *ledgerStore {
	return &ledgerStore{
		claims:  map[string]uuid.UUID{},
		sent:    map[uuid.UUID]uuid.UUID{},
		ownerID: uuid.New(),
	}
}

func ledgerKey(conversationID uuid.UUID, at time.Time) string {
	return conversationID.String() + "|" + at.UTC().Format(time.RFC3339Nano)
}

func (s *ledgerStore) TryClaimWindowRecontact(_ context.Context, _, conversationID uuid.UUID, at time.Time) (bool, uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := ledgerKey(conversationID, at)
	if _, exists := s.claims[k]; exists {
		return false, uuid.Nil, nil // ON CONFLICT DO NOTHING
	}
	id := uuid.New()
	s.claims[k] = id
	return true, id, nil
}

func (s *ledgerStore) MarkWindowRecontactSent(_ context.Context, sendID, messageID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[sendID] = messageID
	return nil
}

func (s *ledgerStore) DeleteWindowRecontactClaim(_ context.Context, sendID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, done := s.sent[sendID]; done {
		return nil // only unsent claims are deletable (message_id IS NULL)
	}
	for k, id := range s.claims {
		if id == sendID {
			delete(s.claims, k)
		}
	}
	return nil
}

func (s *ledgerStore) FindTeamOwnerUserID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return s.ownerID, nil
}

type countingSender struct {
	calls atomic.Int64
	fail  atomic.Bool
	delay time.Duration
}

func (c *countingSender) SendMessage(_ context.Context, _, _ uuid.UUID, _ string, conversationID uuid.UUID, _ message.SendInput) (*message.Message, error) {
	c.calls.Add(1)
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	if c.fail.Load() {
		return nil, errors.New("meta unavailable")
	}
	return &message.Message{ID: uuid.New(), ConversationID: conversationID}, nil
}

func newRecontactService(store WindowRecontactStore, sender WindowRecontactMessageSender) *WindowRecontactService {
	return NewWindowRecontactService(store, nil, nil, sender, nil, nil, nil)
}

func testCandidate() WindowRecontactCandidate {
	return WindowRecontactCandidate{
		TeamID:                uuid.New(),
		ConversationID:        uuid.New(),
		ContactID:             uuid.New(),
		ChannelType:           "whatsapp",
		LastCustomerMessageAt: time.Now().UTC().Add(-23 * time.Hour),
		Text:                  "Halo kak, masih ada yang bisa dibantu?",
		AfterHours:            23,
	}
}

// Several API replicas run the poller at the same time and see the same due
// candidate. The ledger claim must let exactly one of them send.
func TestWindowRecontact_ConcurrentReplicasSendOnce(t *testing.T) {
	t.Parallel()
	store := newLedgerStore()
	sender := &countingSender{delay: 5 * time.Millisecond}
	candidate := testCandidate()

	const replicas = 16
	var wg sync.WaitGroup
	for i := 0; i < replicas; i++ {
		svc := newRecontactService(store, sender) // one service per "replica"
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.processCandidate(context.Background(), candidate); err != nil {
				t.Errorf("processCandidate: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := sender.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 send across %d replicas, got %d", replicas, got)
	}
}

// A later tick for the same customer message must not send again.
func TestWindowRecontact_SentClaimBlocksLaterTicks(t *testing.T) {
	t.Parallel()
	store := newLedgerStore()
	sender := &countingSender{}
	svc := newRecontactService(store, sender)
	candidate := testCandidate()

	for tick := 0; tick < 3; tick++ {
		if err := svc.processCandidate(context.Background(), candidate); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
	}
	if got := sender.calls.Load(); got != 1 {
		t.Fatalf("expected 1 send over 3 ticks, got %d", got)
	}
}

// A failed send releases the claim so the next tick can retry.
func TestWindowRecontact_FailedSendReleasesClaim(t *testing.T) {
	t.Parallel()
	store := newLedgerStore()
	sender := &countingSender{}
	sender.fail.Store(true)
	svc := newRecontactService(store, sender)
	candidate := testCandidate()

	if err := svc.processCandidate(context.Background(), candidate); err == nil {
		t.Fatal("expected send error")
	}
	sender.fail.Store(false)
	if err := svc.processCandidate(context.Background(), candidate); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := sender.calls.Load(); got != 2 {
		t.Fatalf("expected failed attempt + 1 retry = 2 calls, got %d", got)
	}
	if err := svc.processCandidate(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if got := sender.calls.Load(); got != 2 {
		t.Fatalf("expected no further send after success, got %d", got)
	}
}

// A new customer message opens a new window and may be recontacted again.
func TestWindowRecontact_NewCustomerMessageIsNewClaim(t *testing.T) {
	t.Parallel()
	store := newLedgerStore()
	sender := &countingSender{}
	svc := newRecontactService(store, sender)
	candidate := testCandidate()

	if err := svc.processCandidate(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	candidate.LastCustomerMessageAt = candidate.LastCustomerMessageAt.Add(2 * time.Hour)
	if err := svc.processCandidate(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if got := sender.calls.Load(); got != 2 {
		t.Fatalf("expected 2 sends for 2 distinct customer messages, got %d", got)
	}
}
