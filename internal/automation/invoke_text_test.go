package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type invokeRepoStub struct {
	endpoint *Endpoint
	apiKey   *APIKey
}

func (s *invokeRepoStub) CreateEndpoint(context.Context, *Endpoint) error { return nil }
func (s *invokeRepoStub) FindEndpointByID(context.Context, uuid.UUID, string) (*Endpoint, error) {
	return nil, repository.ErrNotFound
}
func (s *invokeRepoStub) FindEndpointByIDGlobal(_ context.Context, id string) (*Endpoint, error) {
	if s.endpoint != nil && s.endpoint.ID == id {
		return s.endpoint, nil
	}
	return nil, repository.ErrNotFound
}
func (s *invokeRepoStub) ListEndpoints(context.Context, uuid.UUID, EndpointListFilter, repository.PageParams) (repository.Page[Endpoint], error) {
	return repository.Page[Endpoint]{}, nil
}
func (s *invokeRepoStub) UpdateEndpoint(context.Context, *Endpoint) error { return nil }
func (s *invokeRepoStub) SoftDeleteEndpoint(context.Context, uuid.UUID, string) error { return nil }
func (s *invokeRepoStub) TouchEndpointUsed(context.Context, string, time.Time) error { return nil }
func (s *invokeRepoStub) SlugExists(context.Context, uuid.UUID, string, string) (bool, error) {
	return false, nil
}
func (s *invokeRepoStub) CreateAPIKey(context.Context, *APIKey) error { return nil }
func (s *invokeRepoStub) FindAPIKeyByID(context.Context, uuid.UUID, string) (*APIKey, error) {
	return nil, repository.ErrNotFound
}
func (s *invokeRepoStub) FindAPIKeyByHash(_ context.Context, hash string) (*APIKey, error) {
	if s.apiKey != nil && s.apiKey.KeyHash == hash {
		return s.apiKey, nil
	}
	return nil, repository.ErrNotFound
}
func (s *invokeRepoStub) ListAPIKeys(context.Context, uuid.UUID) ([]APIKey, error) { return nil, nil }
func (s *invokeRepoStub) UpdateAPIKey(context.Context, *APIKey) error { return nil }
func (s *invokeRepoStub) SoftDeleteAPIKey(context.Context, uuid.UUID, string) error { return nil }
func (s *invokeRepoStub) TouchAPIKeyUsed(context.Context, string, time.Time) error { return nil }
func (s *invokeRepoStub) CountActiveAPIKeys(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s *invokeRepoStub) CreateExecution(context.Context, *Execution) error { return nil }
func (s *invokeRepoStub) CreateWebhookDelivery(context.Context, *WebhookDelivery) error {
	return nil
}
func (s *invokeRepoStub) CreateForwardWebhook(context.Context, *ForwardWebhook) error { return nil }
func (s *invokeRepoStub) FindForwardWebhookByID(context.Context, uuid.UUID, string) (*ForwardWebhook, error) {
	return nil, repository.ErrNotFound
}
func (s *invokeRepoStub) ListForwardWebhooks(context.Context, uuid.UUID) ([]ForwardWebhook, error) {
	return nil, nil
}
func (s *invokeRepoStub) ListActiveForwardWebhooks(context.Context, uuid.UUID) ([]ForwardWebhook, error) {
	return nil, nil
}
func (s *invokeRepoStub) UpdateForwardWebhook(context.Context, *ForwardWebhook) error { return nil }
func (s *invokeRepoStub) SoftDeleteForwardWebhook(context.Context, uuid.UUID, string) error {
	return nil
}
func (s *invokeRepoStub) TouchForwardWebhookUsed(context.Context, string, time.Time) error { return nil }
func (s *invokeRepoStub) CreateForwardDelivery(context.Context, *ForwardDelivery) error { return nil }
func (s *invokeRepoStub) ListForwardDeliveries(context.Context, uuid.UUID, string, repository.PageParams) (repository.Page[ForwardDelivery], error) {
	return repository.Page[ForwardDelivery]{}, nil
}
func (s *invokeRepoStub) GetSummary(context.Context, uuid.UUID) (*Summary, error) { return nil, nil }
func (s *invokeRepoStub) GetEndpointStats(context.Context, string) (EndpointStats, error) { return EndpointStats{}, nil }
func (s *invokeRepoStub) GetEndpointAnalytics(context.Context, uuid.UUID, string) (*EndpointAnalytics, error) {
	return nil, nil
}

type invokeContactsStub struct {
	contact *contact.Contact
	err     error
}

func (s *invokeContactsStub) FindByPhone(context.Context, uuid.UUID, string) (*contact.Contact, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.contact, nil
}

func (s *invokeContactsStub) Create(context.Context, *contact.Contact) error { return nil }

type invokeConversationsStub struct {
	conversation *conversation.Conversation
	err          error
}

func (s *invokeConversationsStub) FindByContactChannel(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*conversation.Conversation, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.conversation, nil
}

func (s *invokeConversationsStub) Create(context.Context, *conversation.Conversation) error { return nil }
func (s *invokeConversationsStub) UpdateLastMessage(context.Context, uuid.UUID, uuid.UUID, conversation.LastMessageUpdate) error {
	return nil
}

type invokeMessagesStub struct {
	items []message.Message
	err   error
}

func (s *invokeMessagesStub) ListByConversation(context.Context, uuid.UUID, uuid.UUID, message.ListFilter, repository.PageParams) (repository.Page[message.Message], error) {
	if s.err != nil {
		return repository.Page[message.Message]{}, s.err
	}
	return repository.Page[message.Message]{Items: s.items}, nil
}

func (s *invokeMessagesStub) Create(context.Context, *message.Message) error { return nil }

type invokeChannelsStub struct {
	channel *channel.Channel
	creds   *channel.Credentials
}

func (s *invokeChannelsStub) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	if s.channel == nil {
		return nil, repository.ErrNotFound
	}
	return s.channel, nil
}

func (s *invokeChannelsStub) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	if s.creds == nil {
		return nil, repository.ErrNotFound
	}
	return s.creds, nil
}

type invokeMetaStub struct {
	lastText string
}

func (s *invokeMetaStub) SendTemplateMessage(context.Context, string, string, string, string, string, []metaclient.TemplateComponent) (string, error) {
	return "", errors.New("not expected")
}

func (s *invokeMetaStub) SendTextMessage(_ context.Context, _ string, _ string, _ string, body string) (string, error) {
	s.lastText = body
	return "wamid.1", nil
}

func TestInvokePublicSendTextHappyPath(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	contactID := uuid.New()
	convID := uuid.New()

	cfg, _ := json.Marshal(SendTextActionConfig{
		ChannelID:     channelID.String(),
		RecipientMode: RecipientModePhone,
		TextParameter: "text",
	})
	endpoint := &Endpoint{
		ID:           "ept_1",
		TeamID:       teamID,
		Status:       EndpointStatusActive,
		ActionType:   ActionSendText,
		ActionConfig: cfg,
	}
	key := &APIKey{
		ID:      "key_1",
		TeamID:  teamID,
		Status:  APIKeyStatusActive,
		Scopes:  []string{"send_text"},
		KeyHash: HashAPIKey("oc_auto_key"),
	}
	cipher, err := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt("meta-token")
	if err != nil {
		t.Fatal(err)
	}
	metaStub := &invokeMetaStub{}
	inv := NewInvoker(
		&invokeRepoStub{endpoint: endpoint, apiKey: key},
		nil,
		&invokeChannelsStub{
			channel: &channel.Channel{
				ID:            channelID,
				TeamID:        teamID,
				ChannelType:   channel.TypeWhatsApp,
				Status:        channel.StatusConnected,
				PhoneNumberID: ptr("12345"),
			},
			creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
		},
		&invokeContactsStub{contact: &contact.Contact{ID: contactID, TeamID: teamID}},
		&invokeConversationsStub{conversation: &conversation.Conversation{
			ID:      convID,
			TeamID:  teamID,
			Status:  conversation.StatusOpen,
			ChannelID: &channelID,
		}},
		&invokeMessagesStub{items: []message.Message{
			{ID: uuid.New(), TeamID: teamID, ConversationID: convID, Direction: message.DirectionInbound, CreatedAt: time.Now().Add(-2 * time.Hour)},
		}},
		cipher,
		metaStub,
	)

	out, err := inv.InvokePublic(context.Background(), endpoint.ID, "Bearer oc_auto_key", map[string]any{
		"to":   "+628123",
		"text": "halo",
	}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if out.StatusCode != 200 || !out.Success {
		t.Fatalf("unexpected result: %#v", out)
	}
	if metaStub.lastText != "halo" {
		t.Fatalf("unexpected text sent: %q", metaStub.lastText)
	}
}

func TestInvokePublicSendTextConversationWindowExpired(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	contactID := uuid.New()
	convID := uuid.New()
	cfg, _ := json.Marshal(SendTextActionConfig{ChannelID: channelID.String(), RecipientMode: RecipientModePhone})
	endpoint := &Endpoint{ID: "ept_1", TeamID: teamID, Status: EndpointStatusActive, ActionType: ActionSendText, ActionConfig: cfg}
	key := &APIKey{ID: "key_1", TeamID: teamID, Status: APIKeyStatusActive, Scopes: []string{"send_text"}, KeyHash: HashAPIKey("oc_auto_key")}
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("meta-token")

	inv := NewInvoker(
		&invokeRepoStub{endpoint: endpoint, apiKey: key},
		nil,
		&invokeChannelsStub{
			channel: &channel.Channel{ID: channelID, TeamID: teamID, ChannelType: channel.TypeWhatsApp, Status: channel.StatusConnected, PhoneNumberID: ptr("12345")},
			creds:   &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
		},
		&invokeContactsStub{contact: &contact.Contact{ID: contactID, TeamID: teamID}},
		&invokeConversationsStub{conversation: &conversation.Conversation{ID: convID, TeamID: teamID, Status: conversation.StatusOpen, ChannelID: &channelID}},
		&invokeMessagesStub{items: []message.Message{
			{ID: uuid.New(), TeamID: teamID, ConversationID: convID, Direction: message.DirectionInbound, CreatedAt: time.Now().Add(-48 * time.Hour).UTC()},
		}},
		cipher,
		&invokeMetaStub{},
	)
	out, err := inv.InvokePublic(context.Background(), endpoint.ID, "Bearer oc_auto_key", map[string]any{"to": "+628123", "text": "halo"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if out.StatusCode != 422 {
		t.Fatalf("expected 422, got %#v", out)
	}
}

func TestInvokePublicTeamIsolation(t *testing.T) {
	endpoint := &Endpoint{
		ID:         "ept_1",
		TeamID:     uuid.New(),
		Status:     EndpointStatusActive,
		ActionType: ActionSendText,
	}
	key := &APIKey{
		ID:      "key_1",
		TeamID:  uuid.New(),
		Status:  APIKeyStatusActive,
		Scopes:  []string{"send_text"},
		KeyHash: HashAPIKey("oc_auto_key"),
	}
	inv := NewInvoker(&invokeRepoStub{endpoint: endpoint, apiKey: key}, nil, nil, nil, nil, nil, nil, nil)
	_, err := inv.InvokePublic(context.Background(), endpoint.ID, "Bearer oc_auto_key", map[string]any{}, "")
	if !errors.Is(err, serviceerrors.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func ptr(s string) *string { return &s }
