package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

// InboxRecorder mirrors successful campaign sends into Inbox conversation history.
type InboxRecorder interface {
	RecordCampaignSend(ctx context.Context, in CampaignInboxRecordInput) error
}

// CampaignInboxRecordInput is one successful WhatsApp template send to mirror.
type CampaignInboxRecordInput struct {
	TeamID           uuid.UUID
	ChannelID        uuid.UUID
	ContactID        uuid.UUID
	CampaignName     string
	Template         *template.Template
	RecipientPhone   string
	Variables        map[string]string
	WAMID            string
	ProviderResponse json.RawMessage
	SentAt           time.Time
}

// ConversationFinder finds or creates the WhatsApp conversation for a contact+channel.
type ConversationFinder interface {
	FindOrCreateForInbound(
		ctx context.Context,
		teamID, contactID, channelID uuid.UUID,
		channelType conversation.ChannelType,
	) (*conversation.Conversation, bool, error)
}

// OutboundMessageIngester stores outbound messages idempotently by WAMID.
type OutboundMessageIngester interface {
	IngestOutboundEcho(ctx context.Context, input message.OutboundEchoInput) (*message.Message, bool, error)
}

// OutboundInboxSync updates conversation preview / realtime after an outbound message.
type OutboundInboxSync interface {
	HandleOutbound(ctx context.Context, p inbox.OutboundParams) (*conversation.Conversation, error)
}

// CampaignInboxRecorder writes campaign sends into messages + inbox sync.
type CampaignInboxRecorder struct {
	conversations ConversationFinder
	messages      OutboundMessageIngester
	inbox         OutboundInboxSync
}

// NewCampaignInboxRecorder wires conversation + message + inbox sync for broadcast→inbox.
func NewCampaignInboxRecorder(
	conversations ConversationFinder,
	messages OutboundMessageIngester,
	inboxSync OutboundInboxSync,
) *CampaignInboxRecorder {
	return &CampaignInboxRecorder{
		conversations: conversations,
		messages:      messages,
		inbox:         inboxSync,
	}
}

func (r *CampaignInboxRecorder) RecordCampaignSend(ctx context.Context, in CampaignInboxRecordInput) error {
	if r == nil || r.conversations == nil || r.messages == nil {
		return fmt.Errorf("inbox recorder is not configured")
	}
	wamid := strings.TrimSpace(in.WAMID)
	if wamid == "" {
		return fmt.Errorf("wamid is required")
	}
	if in.ContactID == uuid.Nil || in.ChannelID == uuid.Nil || in.TeamID == uuid.Nil {
		return fmt.Errorf("team, channel, and contact are required")
	}

	conv, _, err := r.conversations.FindOrCreateForInbound(
		ctx,
		in.TeamID,
		in.ContactID,
		in.ChannelID,
		conversation.ChannelWhatsApp,
	)
	if err != nil {
		return err
	}

	content := renderCampaignInboxContent(in.Template, in.CampaignName, in.Variables)
	sentAt := in.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}

	msg, created, err := r.messages.IngestOutboundEcho(ctx, message.OutboundEchoInput{
		TeamID:         in.TeamID,
		ChannelID:      in.ChannelID,
		ConversationID: conv.ID,
		WAMID:          wamid,
		Sender:         "broadcast",
		Recipient:      strings.TrimSpace(in.RecipientPhone),
		Content:        content,
		ContentType:    message.ContentTemplate,
		RawPayload:     in.ProviderResponse,
		ReceivedAt:     sentAt,
	})
	if err != nil {
		return err
	}
	if !created || r.inbox == nil || msg == nil {
		return nil
	}
	_, err = r.inbox.HandleOutbound(ctx, inbox.OutboundParams{
		TeamID:       in.TeamID,
		AgentID:      uuid.Nil,
		Conversation: conv,
		Message:      msg,
	})
	return err
}

func renderCampaignInboxContent(tpl *template.Template, campaignName string, variables map[string]string) string {
	preview := ""
	if tpl != nil {
		if tpl.Preview != nil {
			preview = strings.TrimSpace(*tpl.Preview)
		}
		if preview == "" {
			preview = template.ExtractPreview(tpl.Components)
		}
	}
	if preview == "" {
		preview = strings.TrimSpace(campaignName)
	}
	if preview == "" {
		preview = "Broadcast message"
	}
	for key, value := range variables {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		preview = strings.ReplaceAll(preview, "{{"+key+"}}", value)
		// Snapshot keys are "body.1" / "header.1"; Meta body text uses "{{1}}".
		if strings.HasPrefix(key, "body.") {
			preview = strings.ReplaceAll(preview, "{{"+strings.TrimPrefix(key, "body.")+"}}", value)
		}
		if strings.HasPrefix(key, "header.") {
			preview = strings.ReplaceAll(preview, "{{"+strings.TrimPrefix(key, "header.")+"}}", value)
		}
	}
	if len(preview) > 500 {
		return preview[:497] + "..."
	}
	return preview
}

// Ensure CampaignInboxRecorder satisfies InboxRecorder.
var _ InboxRecorder = (*CampaignInboxRecorder)(nil)
