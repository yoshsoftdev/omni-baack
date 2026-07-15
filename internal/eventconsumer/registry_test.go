package eventconsumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/consumererrors"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/resolver"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

type fakeChannelRepo struct {
	byPhone map[string]*channel.Channel
}

func (f *fakeChannelRepo) Create(context.Context, *channel.Channel) error { return nil }
func (f *fakeChannelRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (f *fakeChannelRepo) Update(context.Context, *channel.Channel) error { return nil }
func (f *fakeChannelRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeChannelRepo) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f *fakeChannelRepo) UpsertCredentials(context.Context, *channel.Credentials) error { return nil }
func (f *fakeChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) DeleteCredentials(context.Context, uuid.UUID) error { return nil }
func (f *fakeChannelRepo) UpsertOnboarding(context.Context, *channel.Onboarding) error { return nil }
func (f *fakeChannelRepo) FindOnboarding(context.Context, uuid.UUID) (*channel.Onboarding, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f *fakeChannelRepo) FindByPhoneNumberID(_ context.Context, phone string) (*channel.Channel, error) {
	ch, ok := f.byPhone[phone]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return ch, nil
}
func (f *fakeChannelRepo) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) FindByIGUserID(context.Context, uuid.UUID, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) FindByIGUserIDGlobal(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) FindConnectedByType(context.Context, uuid.UUID, channel.ChannelType) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeChannelRepo) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

type fakeContactRepo struct {
	byPhone map[string]*contact.Contact
	created int
}

func (f *fakeContactRepo) Create(_ context.Context, c *contact.Contact) error {
	f.created++
	f.byPhone[c.Phone] = c
	return nil
}
func (f *fakeContactRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*contact.Contact, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeContactRepo) FindByPhone(_ context.Context, _ uuid.UUID, phone string) (*contact.Contact, error) {
	c, ok := f.byPhone[phone]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}
func (f *fakeContactRepo) List(context.Context, uuid.UUID, contact.ListFilter, repository.PageParams) (repository.Page[contact.Contact], error) {
	return repository.Page[contact.Contact]{}, nil
}
func (f *fakeContactRepo) ListByIDs(context.Context, uuid.UUID, []uuid.UUID) ([]contact.Contact, error) {
	return nil, nil
}
func (f *fakeContactRepo) Update(context.Context, *contact.Contact) error { return nil }
func (f *fakeContactRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeContactRepo) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f *fakeContactRepo) ListTagsByContactIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]contact.TagRef, error) {
	return map[uuid.UUID][]contact.TagRef{}, nil
}
func (f *fakeContactRepo) SyncContactTags(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error { return nil }
func (f *fakeContactRepo) CountMarketingByTeam(context.Context, uuid.UUID) (map[contact.MarketingStatus]int64, error) {
	return map[contact.MarketingStatus]int64{}, nil
}
func (f *fakeContactRepo) LoadContactStats(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]contact.ContactStats, error) {
	return map[uuid.UUID]contact.ContactStats{}, nil
}
func (f *fakeContactRepo) ListFieldDefinitions(context.Context, uuid.UUID) ([]contact.FieldDefinition, error) {
	return nil, nil
}
func (f *fakeContactRepo) FindFieldDefinitionByID(context.Context, uuid.UUID, uuid.UUID) (*contact.FieldDefinition, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeContactRepo) CreateFieldDefinition(context.Context, *contact.FieldDefinition) error { return nil }
func (f *fakeContactRepo) UpdateFieldDefinition(context.Context, *contact.FieldDefinition) error { return nil }
func (f *fakeContactRepo) SoftDeleteFieldDefinition(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeContactRepo) ListCustomFieldsByContactIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]contact.CustomFieldValue, error) {
	return map[uuid.UUID][]contact.CustomFieldValue{}, nil
}
func (f *fakeContactRepo) UpsertCustomFieldValues(context.Context, uuid.UUID, map[uuid.UUID]json.RawMessage) error {
	return nil
}

type fakeConversationRepo struct {
	items map[uuid.UUID]*conversation.Conversation
	created int
}

func (f *fakeConversationRepo) Create(_ context.Context, c *conversation.Conversation) error {
	f.created++
	f.items[c.ID] = c
	return nil
}
func (f *fakeConversationRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*conversation.Conversation, error) {
	c, ok := f.items[id]
	if !ok || c.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	return c, nil
}
func (f *fakeConversationRepo) FindByContactChannel(_ context.Context, teamID, contactID uuid.UUID, channelID *uuid.UUID) (*conversation.Conversation, error) {
	for _, c := range f.items {
		if c.TeamID == teamID && c.ContactID == contactID && c.ChannelID != nil && channelID != nil && *c.ChannelID == *channelID {
			return c, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeConversationRepo) List(context.Context, uuid.UUID, conversation.ListFilter, repository.PageParams) (repository.Page[conversation.Conversation], error) {
	return repository.Page[conversation.Conversation]{}, nil
}
func (f *fakeConversationRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, conversation.Status) error {
	return nil
}
func (f *fakeConversationRepo) UpdateAssignment(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error {
	return nil
}
func (f *fakeConversationRepo) UpdateUnreadCount(context.Context, uuid.UUID, uuid.UUID, int) error { return nil }
func (f *fakeConversationRepo) UpdateLastMessage(context.Context, uuid.UUID, uuid.UUID, conversation.LastMessageUpdate) error {
	return nil
}
func (f *fakeConversationRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeConversationRepo) ApplyInboxSync(context.Context, uuid.UUID, uuid.UUID, conversation.InboxSyncUpdate) error {
	return nil
}
func (f *fakeConversationRepo) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeConversationRepo) CountOpenByAssignee(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
func (f *fakeConversationRepo) CountByTeamFilter(context.Context, uuid.UUID, conversation.ListFilter) (int64, error) {
	return 0, nil
}

type fakeMessageRepo struct {
	byWAMID map[string]*message.Message
	created int
}

func (f *fakeMessageRepo) Create(_ context.Context, m *message.Message) error {
	f.created++
	if m.WAMID != nil {
		f.byWAMID[*m.WAMID] = m
	}
	return nil
}
func (f *fakeMessageRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*message.Message, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeMessageRepo) FindByWAMID(_ context.Context, wamid string) (*message.Message, error) {
	m, ok := f.byWAMID[wamid]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return m, nil
}
func (f *fakeMessageRepo) FindByWAMIDKey(context.Context, uuid.UUID, uuid.UUID, string) (*message.Message, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeMessageRepo) ListByConversation(context.Context, uuid.UUID, uuid.UUID, message.ListFilter, repository.PageParams) (repository.Page[message.Message], error) {
	return repository.Page[message.Message]{}, nil
}
func (f *fakeMessageRepo) UpdateStatus(_ context.Context, _ uuid.UUID, id uuid.UUID, status message.Status) error {
	for _, m := range f.byWAMID {
		if m.ID == id {
			s := status
			m.Status = &s
		}
	}
	return nil
}

func (f *fakeMessageRepo) MarkDeletedByID(_ context.Context, _ uuid.UUID, id uuid.UUID, source message.DeletedBySource) error {
	for _, m := range f.byWAMID {
		if m.ID == id {
			m.IsDeleted = true
			s := source
			m.DeletedBySource = &s
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeMessageRepo) MarkDeletedByWAMID(_ context.Context, _ uuid.UUID, wamid string, source message.DeletedBySource) error {
	m, ok := f.byWAMID[wamid]
	if !ok {
		return repository.ErrNotFound
	}
	m.IsDeleted = true
	s := source
	m.DeletedBySource = &s
	return nil
}

type fakeTeamRepo struct{}

func (fakeTeamRepo) FindMemberByUser(context.Context, uuid.UUID, uuid.UUID) (*struct{}, error) {
	return nil, errors.New("unused")
}

type noopDLQ struct{}

func (noopDLQ) PublishConsumerDLQ(context.Context, string, string, sharedevents.CanonicalEvent) error {
	return nil
}

func TestRegistry_UnknownPhoneNumberIDRoutesTerminal(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	chRepo := &fakeChannelRepo{byPhone: map[string]*channel.Channel{}}
	registry := eventconsumer.NewRegistry(eventconsumer.Deps{
		Channels:    chRepo,
		Contacts:    contact.NewService(&fakeContactRepo{byPhone: map[string]*contact.Contact{}}, nil, nil, nil),
		Conversations: conversation.NewService(&fakeConversationRepo{items: map[uuid.UUID]*conversation.Conversation{}}, &fakeContactRepo{byPhone: map[string]*contact.Contact{}}, nil, nil),
		Messages:    message.NewService(&fakeMessageRepo{byWAMID: map[string]*message.Message{}}, nil, &fakeConversationRepo{items: map[uuid.UUID]*conversation.Conversation{}}, nil, nil, nil, nil),
		Idempotency: idempotency.NewMemoryStore(time.Hour),
		DLQ:         noopDLQ{},
	})

	payload, _ := json.Marshal(sharedevents.MetaMessagePayload{
		PhoneNumberID: "unknown-phone",
		WAMID:         "wamid-1",
		From:          "628111",
		MessageType:   "text",
		Raw:           json.RawMessage(`{"text":{"body":"hi"}}`),
	})
	err := registry.HandleCanonical(context.Background(), sharedevents.CanonicalEvent{
		EventID:  uuid.NewString(),
		Provider: sharedevents.ProviderMeta,
		Type:     sharedevents.TypeMessageReceived,
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("expected ack after dlq, got %v", err)
	}
	_ = teamID
	_ = channelID
}

func TestMessageHandler_NewConversationAndMessage(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	phoneID := "phone-123"
	chRepo := &fakeChannelRepo{byPhone: map[string]*channel.Channel{
		phoneID: {ID: channelID, TeamID: teamID, PhoneNumberID: &phoneID},
	}}
	contactRepo := &fakeContactRepo{byPhone: map[string]*contact.Contact{}}
	convRepo := &fakeConversationRepo{items: map[uuid.UUID]*conversation.Conversation{}}
	msgRepo := &fakeMessageRepo{byWAMID: map[string]*message.Message{}}

	metrics := &eventconsumer.Metrics{}
	handler := eventconsumer.NewMessageHandler(
		resolver.NewTenantResolver(chRepo),
		chRepo,
		contact.NewService(contactRepo, nil, nil, nil),
		conversation.NewService(convRepo, contactRepo, nil, nil),
		message.NewService(msgRepo, nil, convRepo, nil, nil, nil, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		idempotency.NewMemoryStore(time.Hour),
		metrics,
		nil,
	)

	payload, _ := json.Marshal(sharedevents.MetaMessagePayload{
		PhoneNumberID: phoneID,
		WAMID:         "wamid-abc",
		From:          "628123456789",
		ContactName:   "Alice",
		MessageType:   "text",
		Raw:           json.RawMessage(`{"text":{"body":"hello"}}`),
	})
	if err := handler.Handle(context.Background(), sharedevents.CanonicalEvent{
		EventID:    uuid.NewString(),
		Provider:   sharedevents.ProviderMeta,
		Type:       sharedevents.TypeMessageReceived,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if contactRepo.created != 1 {
		t.Fatalf("expected auto contact, got %d", contactRepo.created)
	}
	if convRepo.created != 1 {
		t.Fatalf("expected new conversation, got %d", convRepo.created)
	}
	if msgRepo.created != 1 {
		t.Fatalf("expected message created, got %d", msgRepo.created)
	}
}

func TestMessageHandler_DuplicateSkipped(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	phoneID := "phone-123"
	chRepo := &fakeChannelRepo{byPhone: map[string]*channel.Channel{
		phoneID: {ID: channelID, TeamID: teamID, PhoneNumberID: &phoneID},
	}}
	contactRepo := &fakeContactRepo{byPhone: map[string]*contact.Contact{}}
	convRepo := &fakeConversationRepo{items: map[uuid.UUID]*conversation.Conversation{}}
	msgRepo := &fakeMessageRepo{byWAMID: map[string]*message.Message{}}

	handler := eventconsumer.NewMessageHandler(
		resolver.NewTenantResolver(chRepo),
		chRepo,
		contact.NewService(contactRepo, nil, nil, nil),
		conversation.NewService(convRepo, contactRepo, nil, nil),
		message.NewService(msgRepo, nil, convRepo, nil, nil, nil, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		idempotency.NewMemoryStore(time.Hour),
		&eventconsumer.Metrics{},
		nil,
	)

	payload, _ := json.Marshal(sharedevents.MetaMessagePayload{
		PhoneNumberID: phoneID,
		WAMID:         "wamid-dup",
		From:          "628123456789",
		MessageType:   "text",
		Raw:           json.RawMessage(`{"text":{"body":"hello"}}`),
	})
	event := sharedevents.CanonicalEvent{
		EventID:  uuid.NewString(),
		Provider: sharedevents.ProviderMeta,
		Type:     sharedevents.TypeMessageReceived,
		Payload:  payload,
	}
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if msgRepo.created != 1 {
		t.Fatalf("expected single message, got %d", msgRepo.created)
	}
}

func TestStatusHandler_Progression(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	phoneID := "phone-123"
	chRepo := &fakeChannelRepo{byPhone: map[string]*channel.Channel{
		phoneID: {ID: channelID, TeamID: teamID, PhoneNumberID: &phoneID},
	}}
	msgRepo := &fakeMessageRepo{byWAMID: map[string]*message.Message{}}
	sent := message.StatusSent
	msgRepo.byWAMID["wamid-status"] = &message.Message{
		ID:     uuid.New(),
		TeamID: teamID,
		WAMID:  strPtr("wamid-status"),
		Status: &sent,
	}

	handler := eventconsumer.NewStatusHandler(
		resolver.NewTenantResolver(chRepo),
		message.NewService(msgRepo, nil, nil, nil, nil, nil, nil),
		nil,
		idempotency.NewMemoryStore(time.Hour),
		&eventconsumer.Metrics{},
		nil,
	)

	for _, eventType := range []string{
		sharedevents.TypeMessageStatusDelivered,
		sharedevents.TypeMessageStatusRead,
		sharedevents.TypeMessageStatusSent,
	} {
		payload, _ := json.Marshal(sharedevents.MetaStatusPayload{
			PhoneNumberID: phoneID,
			WAMID:         "wamid-status",
			Status:        "read",
		})
		_ = handler.Handle(context.Background(), sharedevents.CanonicalEvent{
			EventID:  uuid.NewString(),
			Provider: sharedevents.ProviderMeta,
			Type:     eventType,
			Payload:  payload,
		})
	}
	final := msgRepo.byWAMID["wamid-status"].Status
	if final == nil || *final != message.StatusRead {
		t.Fatalf("expected read status, got %v", final)
	}
}

func TestTenantResolver_UnknownPhone(t *testing.T) {
	r := resolver.NewTenantResolver(&fakeChannelRepo{byPhone: map[string]*channel.Channel{}})
	_, err := r.ResolveByPhoneNumberID(context.Background(), "missing")
	te, ok := consumererrors.IsTerminal(err)
	if !ok || te.Category != "channel" {
		t.Fatalf("expected terminal channel error, got %v", err)
	}
}

func strPtr(s string) *string { return &s }
