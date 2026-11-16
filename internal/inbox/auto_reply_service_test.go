package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

type fakeAutoReplyStore struct {
	welcome  WelcomeSettings
	rules    []KeywordRule
	claimed  map[string]bool // inbound message id + kind
	released []uuid.UUID
	sent     map[uuid.UUID]uuid.UUID
}

func newFakeAutoReplyStore() *fakeAutoReplyStore {
	return &fakeAutoReplyStore{claimed: map[string]bool{}, sent: map[uuid.UUID]uuid.UUID{}}
}

func (f *fakeAutoReplyStore) GetWelcomeSettings(context.Context, uuid.UUID) (WelcomeSettings, error) {
	return f.welcome, nil
}
func (f *fakeAutoReplyStore) UpsertWelcomeSettings(_ context.Context, _ uuid.UUID, s WelcomeSettings) error {
	f.welcome = s
	return nil
}
func (f *fakeAutoReplyStore) ListKeywordRules(context.Context, uuid.UUID) ([]KeywordRule, error) {
	return f.rules, nil
}
func (f *fakeAutoReplyStore) CountKeywordRules(context.Context, uuid.UUID) (int, error) {
	return len(f.rules), nil
}
func (f *fakeAutoReplyStore) CreateKeywordRule(_ context.Context, _ uuid.UUID, r *KeywordRule) error {
	f.rules = append(f.rules, *r)
	return nil
}
func (f *fakeAutoReplyStore) UpdateKeywordRule(context.Context, uuid.UUID, *KeywordRule) error {
	return nil
}
func (f *fakeAutoReplyStore) DeleteKeywordRule(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeAutoReplyStore) TryClaimAutoReply(_ context.Context, _, _, inbound uuid.UUID, kind string, _ *uuid.UUID) (bool, uuid.UUID, error) {
	key := inbound.String() + kind
	if f.claimed[key] {
		return false, uuid.Nil, nil
	}
	f.claimed[key] = true
	return true, uuid.New(), nil
}
func (f *fakeAutoReplyStore) MarkAutoReplySent(_ context.Context, claim, msg uuid.UUID) error {
	f.sent[claim] = msg
	return nil
}
func (f *fakeAutoReplyStore) DeleteAutoReplyClaim(_ context.Context, claim uuid.UUID) error {
	f.released = append(f.released, claim)
	return nil
}
func (f *fakeAutoReplyStore) FindTeamOwnerUserID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

type fakeSender struct {
	texts []string
	fail  bool
}

func (f *fakeSender) SendMessage(_ context.Context, _, _ uuid.UUID, _ string, conv uuid.UUID, in message.SendInput) (*message.Message, error) {
	if f.fail {
		return nil, errors.New("meta down")
	}
	f.texts = append(f.texts, in.Content)
	return &message.Message{ID: uuid.New(), ConversationID: conv, Content: in.Content}, nil
}

type fakeConvStore struct{}

func (fakeConvStore) FindByID(_ context.Context, _, id uuid.UUID) (*conversation.Conversation, error) {
	return &conversation.Conversation{ID: id}, nil
}

type fakeOutboundSync struct{ automated []bool }

func (f *fakeOutboundSync) HandleOutbound(_ context.Context, p OutboundParams) (*conversation.Conversation, error) {
	f.automated = append(f.automated, p.Automated)
	return p.Conversation, nil
}

func newTestAutoReply(store *fakeAutoReplyStore, sender *fakeSender, sync *fakeOutboundSync) *AutoReplyService {
	return NewAutoReplyService(store, nil, nil, sender, fakeConvStore{}, sync, nil)
}

func inboundText(text string) *message.Message {
	return &message.Message{ID: uuid.New(), Content: text, ContentType: message.ContentText, ReceivedAt: time.Now()}
}

func TestAutoReplyWelcomeAndKeywordOnFirstMessage(t *testing.T) {
	store := newFakeAutoReplyStore()
	store.welcome = WelcomeSettings{Enabled: true, Text: "Selamat datang!"}
	store.rules = []KeywordRule{{ID: uuid.New(), Keywords: []string{"harga"}, MatchType: MatchContains, ReplyText: "Daftar harga: ...", Enabled: true}}
	sender, sync := &fakeSender{}, &fakeOutboundSync{}
	svc := newTestAutoReply(store, sender, sync)
	in := AutoReplyInbound{TeamID: uuid.New(), ConversationID: uuid.New(), ConversationCreated: true, Message: inboundText("Berapa harga?")}

	svc.HandleInbound(context.Background(), in)
	if len(sender.texts) != 2 || sender.texts[0] != "Selamat datang!" || sender.texts[1] != "Daftar harga: ..." {
		t.Fatalf("sent %v, want welcome then keyword reply", sender.texts)
	}
	if len(sync.automated) != 2 || !sync.automated[0] || !sync.automated[1] {
		t.Fatalf("outbound sync automated = %v, want all true (not a human response)", sync.automated)
	}

	// The same inbound message again (event redelivered): the ledger prevents a second send.
	svc.HandleInbound(context.Background(), in)
	if len(sender.texts) != 2 {
		t.Fatalf("redelivery sent again: %v", sender.texts)
	}
}

func TestAutoReplyQuietWhileHumanActiveAndForNonText(t *testing.T) {
	store := newFakeAutoReplyStore()
	store.welcome = WelcomeSettings{Enabled: true, Text: "Selamat datang!"}
	store.rules = []KeywordRule{{ID: uuid.New(), Keywords: []string{"harga"}, MatchType: MatchContains, ReplyText: "price", Enabled: true}}
	sender := &fakeSender{}
	svc := newTestAutoReply(store, sender, &fakeOutboundSync{})

	recent := time.Now().Add(-time.Hour)
	svc.HandleInbound(context.Background(), AutoReplyInbound{TeamID: uuid.New(), ConversationID: uuid.New(), LastHumanReplyAt: &recent, Message: inboundText("harga")})
	image := &message.Message{ID: uuid.New(), Content: "harga", ContentType: message.ContentImage, ReceivedAt: time.Now()}
	svc.HandleInbound(context.Background(), AutoReplyInbound{TeamID: uuid.New(), ConversationID: uuid.New(), Message: image})
	if len(sender.texts) != 0 {
		t.Fatalf("sent %v, want nothing (agent replied an hour ago; image message)", sender.texts)
	}

	old := time.Now().Add(-25 * time.Hour)
	svc.HandleInbound(context.Background(), AutoReplyInbound{TeamID: uuid.New(), ConversationID: uuid.New(), LastHumanReplyAt: &old, Message: inboundText("harga")})
	if len(sender.texts) != 1 || sender.texts[0] != "price" {
		t.Fatalf("sent %v, want the keyword reply once the hand-over window passed", sender.texts)
	}
}

func TestAutoReplyFailedSendReleasesClaim(t *testing.T) {
	store := newFakeAutoReplyStore()
	store.welcome = WelcomeSettings{Enabled: true, Text: "hi"}
	svc := newTestAutoReply(store, &fakeSender{fail: true}, &fakeOutboundSync{})

	svc.HandleInbound(context.Background(), AutoReplyInbound{TeamID: uuid.New(), ConversationID: uuid.New(), ConversationCreated: true, Message: inboundText("x")})
	if len(store.released) != 1 || len(store.sent) != 0 {
		t.Fatalf("released=%d sent=%d, want the claim released so a retry is possible", len(store.released), len(store.sent))
	}
}
