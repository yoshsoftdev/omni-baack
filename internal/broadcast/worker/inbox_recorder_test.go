package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

type mockConversationFinder struct {
	conv    *conversation.Conversation
	created bool
	err     error
	calls   int
}

func (m *mockConversationFinder) FindOrCreateForInbound(
	_ context.Context,
	teamID, contactID, channelID uuid.UUID,
	channelType conversation.ChannelType,
) (*conversation.Conversation, bool, error) {
	m.calls++
	if m.err != nil {
		return nil, false, m.err
	}
	if m.conv == nil {
		m.conv = &conversation.Conversation{
			ID:          uuid.New(),
			TeamID:      teamID,
			ContactID:   contactID,
			ChannelID:   &channelID,
			ChannelType: channelType,
			Status:      conversation.StatusOpen,
		}
	}
	return m.conv, m.created, nil
}

type mockOutboundIngester struct {
	msg     *message.Message
	created bool
	err     error
	last    message.OutboundEchoInput
	calls   int
}

func (m *mockOutboundIngester) IngestOutboundEcho(_ context.Context, input message.OutboundEchoInput) (*message.Message, bool, error) {
	m.calls++
	m.last = input
	if m.err != nil {
		return nil, false, m.err
	}
	if m.msg == nil {
		m.msg = &message.Message{
			ID:             uuid.New(),
			ConversationID: input.ConversationID,
			TeamID:         input.TeamID,
			Content:        input.Content,
			ContentType:    input.ContentType,
			Direction:      message.DirectionOutbound,
		}
	}
	return m.msg, m.created, nil
}

type mockOutboundSync struct {
	err   error
	calls int
	last  inbox.OutboundParams
}

func (m *mockOutboundSync) HandleOutbound(_ context.Context, p inbox.OutboundParams) (*conversation.Conversation, error) {
	m.calls++
	m.last = p
	if m.err != nil {
		return nil, m.err
	}
	return p.Conversation, nil
}

func TestCampaignInboxRecorder_RecordCampaignSend(t *testing.T) {
	teamID := uuid.New()
	channelID := uuid.New()
	contactID := uuid.New()
	preview := "Halo {{1}}, order {{2}}"
	tpl := &template.Template{Preview: &preview}

	convs := &mockConversationFinder{created: true}
	msgs := &mockOutboundIngester{created: true}
	sync := &mockOutboundSync{}
	rec := NewCampaignInboxRecorder(convs, msgs, sync)

	err := rec.RecordCampaignSend(context.Background(), CampaignInboxRecordInput{
		TeamID:         teamID,
		ChannelID:      channelID,
		ContactID:      contactID,
		CampaignName:   "Promo Juli",
		Template:       tpl,
		RecipientPhone: "628123456789",
		Variables:      map[string]string{"body.1": "Ada", "body.2": "42"},
		WAMID:          "wamid.abc",
		ProviderResponse: json.RawMessage(`{"wamid":"wamid.abc"}`),
		SentAt:         time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordCampaignSend: %v", err)
	}
	if convs.calls != 1 || msgs.calls != 1 || sync.calls != 1 {
		t.Fatalf("calls conv=%d msg=%d sync=%d", convs.calls, msgs.calls, sync.calls)
	}
	if msgs.last.Content != "Halo Ada, order 42" {
		t.Fatalf("content = %q", msgs.last.Content)
	}
	if msgs.last.ContentType != message.ContentTemplate {
		t.Fatalf("content type = %s", msgs.last.ContentType)
	}
	if msgs.last.Sender != "broadcast" {
		t.Fatalf("sender = %s", msgs.last.Sender)
	}
	if sync.last.Message == nil || sync.last.Conversation == nil {
		t.Fatal("sync missing message/conversation")
	}
}

func TestCampaignInboxRecorder_SkipsSyncWhenEchoAlreadyExists(t *testing.T) {
	convs := &mockConversationFinder{}
	msgs := &mockOutboundIngester{created: false, msg: &message.Message{ID: uuid.New()}}
	sync := &mockOutboundSync{}
	rec := NewCampaignInboxRecorder(convs, msgs, sync)

	err := rec.RecordCampaignSend(context.Background(), CampaignInboxRecordInput{
		TeamID:    uuid.New(),
		ChannelID: uuid.New(),
		ContactID: uuid.New(),
		WAMID:     "wamid.dup",
	})
	if err != nil {
		t.Fatalf("RecordCampaignSend: %v", err)
	}
	if sync.calls != 0 {
		t.Fatalf("expected no sync on duplicate echo, got %d", sync.calls)
	}
}

func TestCampaignInboxRecorder_RequiresWAMID(t *testing.T) {
	rec := NewCampaignInboxRecorder(&mockConversationFinder{}, &mockOutboundIngester{}, &mockOutboundSync{})
	err := rec.RecordCampaignSend(context.Background(), CampaignInboxRecordInput{
		TeamID:    uuid.New(),
		ChannelID: uuid.New(),
		ContactID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignInboxRecorder_PropagatesConversationError(t *testing.T) {
	convs := &mockConversationFinder{err: errors.New("db down")}
	rec := NewCampaignInboxRecorder(convs, &mockOutboundIngester{}, &mockOutboundSync{})
	err := rec.RecordCampaignSend(context.Background(), CampaignInboxRecordInput{
		TeamID:    uuid.New(),
		ChannelID: uuid.New(),
		ContactID: uuid.New(),
		WAMID:     "wamid.x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderCampaignInboxContent(t *testing.T) {
	preview := "Hi {{1}}"
	got := renderCampaignInboxContent(
		&template.Template{Preview: &preview},
		"Camp",
		map[string]string{"body.1": "Ayu"},
	)
	if got != "Hi Ayu" {
		t.Fatalf("got %q", got)
	}
	got = renderCampaignInboxContent(nil, "Flash Sale", nil)
	if got != "Flash Sale" {
		t.Fatalf("fallback got %q", got)
	}
}
