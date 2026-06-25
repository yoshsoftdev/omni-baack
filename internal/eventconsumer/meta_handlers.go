package eventconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/resolver"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// MessageHandler processes meta.message.received events.
// Syncer applies inbox synchronization after message ingestion.
type Syncer interface {
	HandleInbound(ctx context.Context, p inbox.InboundParams) (*conversation.Conversation, error)
}

type MessageHandler struct {
	resolver      *resolver.TenantResolver
	contacts      *contact.Service
	conversations *conversation.Service
	messages      *message.Service
	inbox         Syncer
	idempotency   idempotency.Store
	metrics       *Metrics
	log           *slog.Logger
}

func NewMessageHandler(
	resolver *resolver.TenantResolver,
	contacts *contact.Service,
	conversations *conversation.Service,
	messages *message.Service,
	inbox Syncer,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return &MessageHandler{
		resolver:      resolver,
		contacts:      contacts,
		conversations: conversations,
		messages:      messages,
		inbox:         inbox,
		idempotency:   store,
		metrics:       metrics,
		log:           log,
	}
}

func (h *MessageHandler) Handle(ctx context.Context, event sharedevents.CanonicalEvent) error {
	payload, err := message.DecodeMetaMessagePayload(event.Payload)
	if err != nil || payload.WAMID == "" {
		return TerminalDLQ("message", ErrMalformedEvent)
	}

	tenant, err := h.resolver.ResolveByPhoneNumberID(ctx, payload.PhoneNumberID)
	if err != nil {
		return err
	}

	key := idempotency.MessageKey(event.Provider, payload.WAMID)
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		return h.process(ctx, event, tenant, payload)
	})
	if err != nil {
		return err
	}
	if skipped {
		h.log.Info("duplicate inbound message skipped", "event_id", event.EventID, "wamid", payload.WAMID)
	}
	return nil
}

func (h *MessageHandler) process(ctx context.Context, event sharedevents.CanonicalEvent, tenant *resolver.TenantContext, payload sharedevents.MetaMessagePayload) error {
	phone := payload.ContactWAID
	if phone == "" {
		phone = payload.From
	}
	contactEntity, err := h.contacts.FindOrCreateFromInbound(ctx, tenant.TeamID, phone, payload.ContactName)
	if err != nil {
		return err
	}

	conv, convCreated, err := h.conversations.FindOrCreateForInbound(ctx, tenant.TeamID, contactEntity.ID, tenant.ChannelID)
	if err != nil {
		return err
	}
	if convCreated {
		h.metrics.IncConversationsCreated()
	}

	content := message.ExtractTextContent(payload.MessageType, payload.Raw)
	contentType := message.ContentTypeFromMeta(payload.MessageType)
	receivedAt := message.ParseMetaTimestamp(payload.Timestamp)
	if !event.OccurredAt.IsZero() {
		receivedAt = event.OccurredAt
	}

	msg, msgCreated, err := h.messages.IngestInbound(ctx, message.InboundInput{
		TeamID:         tenant.TeamID,
		ChannelID:      tenant.ChannelID,
		ConversationID: conv.ID,
		WAMID:          payload.WAMID,
		Sender:         phone,
		Content:        content,
		ContentType:    contentType,
		RawPayload:     payload.Raw,
		ReceivedAt:     receivedAt,
	})
	if err != nil {
		return err
	}
	if msgCreated {
		h.metrics.IncMessagesCreated()
	}
	if h.inbox != nil {
		if _, err := h.inbox.HandleInbound(ctx, inbox.InboundParams{
			TeamID:              tenant.TeamID,
			Conversation:        conv,
			ConversationCreated: convCreated,
			Message:             msg,
			MessageCreated:      msgCreated,
			Preview:             content,
		}); err != nil {
			return err
		}
	} else if msgCreated {
		unread := conv.UnreadCount + 1
		if err := h.conversations.TouchInbound(ctx, tenant.TeamID, conv.ID, content, unread); err != nil {
			return err
		}
	}

	if h.log == nil {
		h.log = slog.Default()
	}
	h.log.Info("inbound message ingested",
		"event_id", event.EventID,
		"team_id", tenant.TeamID.String(),
		"channel_id", tenant.ChannelID.String(),
		"conversation_id", conv.ID.String(),
		"message_id", msg.ID.String(),
		"event_type", event.Type,
	)
	return nil
}

// CampaignDelivery updates campaign recipient delivery state from status webhooks.
type CampaignDelivery interface {
	HandleStatus(ctx context.Context, teamID uuid.UUID, eventType, wamid, failureText string, rawPayload []byte, at time.Time) error
}

// StatusHandler processes meta.message.status.* events.
type StatusHandler struct {
	resolver    *resolver.TenantResolver
	messages    *message.Service
	delivery    CampaignDelivery
	idempotency idempotency.Store
	metrics     *Metrics
	log         *slog.Logger
}

func NewStatusHandler(
	resolver *resolver.TenantResolver,
	messages *message.Service,
	delivery CampaignDelivery,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *StatusHandler {
	if log == nil {
		log = slog.Default()
	}
	return &StatusHandler{resolver: resolver, messages: messages, delivery: delivery, idempotency: store, metrics: metrics, log: log}
}

func (h *StatusHandler) Handle(ctx context.Context, event sharedevents.CanonicalEvent) error {
	payload, err := message.DecodeMetaStatusPayload(event.Payload)
	if err != nil || payload.WAMID == "" {
		return TerminalDLQ("status", ErrMalformedEvent)
	}
	tenant, err := h.resolver.ResolveByPhoneNumberID(ctx, payload.PhoneNumberID)
	if err != nil {
		return err
	}

	status := mapStatus(event.Type)
	if status == "" {
		return TerminalDLQ("status", ErrMalformedEvent)
	}

	key := idempotency.StatusKey(event.Provider, payload.WAMID, string(status))
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		if err := h.messages.ApplyDeliveryStatus(ctx, payload.WAMID, status); err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return err
			}
		}
		if h.delivery != nil {
			at := event.OccurredAt
			if at.IsZero() {
				at = time.Now().UTC()
			}
			return h.delivery.HandleStatus(ctx, tenant.TeamID, event.Type, payload.WAMID, "", payload.Raw, at)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}
	h.metrics.IncStatusUpdates()
	h.log.Info("message status updated",
		"event_id", event.EventID,
		"wamid", payload.WAMID,
		"status", status,
		"event_type", event.Type,
	)
	return nil
}

func mapStatus(eventType string) message.Status {
	switch eventType {
	case sharedevents.TypeMessageStatusSent:
		return message.StatusSent
	case sharedevents.TypeMessageStatusDelivered:
		return message.StatusDelivered
	case sharedevents.TypeMessageStatusRead:
		return message.StatusRead
	case sharedevents.TypeMessageStatusFailed:
		return message.StatusFailed
	default:
		return ""
	}
}

// TemplateHandler processes meta.template.updated events.
type TemplateHandler struct {
	resolver    *resolver.TenantResolver
	channels    channel.Repository
	templates   template.Repository
	idempotency idempotency.Store
	metrics     *Metrics
	log         *slog.Logger
}

func NewTemplateHandler(
	resolver *resolver.TenantResolver,
	channels channel.Repository,
	templates template.Repository,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *TemplateHandler {
	if log == nil {
		log = slog.Default()
	}
	return &TemplateHandler{
		resolver:    resolver,
		channels:    channels,
		templates:   templates,
		idempotency: store,
		metrics:     metrics,
		log:         log,
	}
}

func (h *TemplateHandler) Handle(ctx context.Context, event sharedevents.CanonicalEvent) error {
	var payload sharedevents.MetaTemplatePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TemplateID == "" {
		return TerminalDLQ("template", ErrMalformedEvent)
	}

	tenant, err := h.resolver.ResolveByWABAID(ctx, payload.WABAID)
	if err != nil {
		return err
	}

	key := idempotency.TemplateKey(event.Provider, payload.TemplateID, event.OccurredAt.Unix())
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		status := mapTemplateStatus(payload.Event)
		return h.templates.UpdateFromWebhook(ctx, tenant.TeamID, tenant.ChannelID, payload.TemplateID, payload.TemplateName, status, nil, event.OccurredAt)
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.log.Info("template webhook skipped: not synced yet", "template_id", payload.TemplateID)
			return nil
		}
		return err
	}
	if skipped {
		return nil
	}
	h.log.Info("template updated from webhook",
		"event_id", event.EventID,
		"team_id", tenant.TeamID.String(),
		"channel_id", tenant.ChannelID.String(),
		"template_id", payload.TemplateID,
	)
	return nil
}

func mapTemplateStatus(event string) template.Status {
	switch strings.ToUpper(strings.TrimSpace(event)) {
	case "APPROVED":
		return template.StatusApproved
	case "REJECTED":
		return template.StatusRejected
	default:
		return template.StatusPending
	}
}

// ChannelHandler processes meta.channel.updated events.
type ChannelHandler struct {
	resolver    *resolver.TenantResolver
	channels    channel.Repository
	idempotency idempotency.Store
	metrics     *Metrics
	log         *slog.Logger
}

func NewChannelHandler(
	resolver *resolver.TenantResolver,
	channels channel.Repository,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *ChannelHandler {
	if log == nil {
		log = slog.Default()
	}
	return &ChannelHandler{resolver: resolver, channels: channels, idempotency: store, metrics: metrics, log: log}
}

func (h *ChannelHandler) Handle(ctx context.Context, event sharedevents.CanonicalEvent) error {
	var payload sharedevents.MetaChannelPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return TerminalDLQ("channel", ErrMalformedEvent)
	}

	tenant, err := h.resolver.ResolveByPhoneNumberID(ctx, payload.PhoneNumberID)
	if err != nil {
		tenant, err = h.resolver.ResolveByWABAID(ctx, payload.WABAID)
		if err != nil {
			return err
		}
	}

	key := idempotency.ChannelKey(event.Provider, payload.Field, payload.PhoneNumberID, event.OccurredAt.Unix())
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		meta := map[string]any{
			"field":           payload.Field,
			"phone_number_id": payload.PhoneNumberID,
			"waba_id":         payload.WABAID,
			"occurred_at":     event.OccurredAt,
			"raw":             json.RawMessage(payload.Raw),
		}
		data, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return h.channels.UpdateHealthMetadata(ctx, tenant.ChannelID, data)
	})
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}
	h.log.Info("channel health updated",
		"event_id", event.EventID,
		"team_id", tenant.TeamID.String(),
		"channel_id", tenant.ChannelID.String(),
		"field", payload.Field,
	)
	return nil
}
