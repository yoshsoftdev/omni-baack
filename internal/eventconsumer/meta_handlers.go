package eventconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
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
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
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
	channels      channel.Repository
	contacts      *contact.Service
	conversations *conversation.Service
	messages      *message.Service
	inbox         Syncer
	metaMedia     MetaMediaClient
	tokenCipher   TokenCipher
	storage       StorageUploader
	idempotency   idempotency.Store
	metrics       *Metrics
	log           *slog.Logger
}

type MetaMediaClient interface {
	GetMediaMetadata(ctx context.Context, mediaID, phoneNumberID, accessToken string) (*metaclient.MediaMetadata, error)
	DownloadMedia(ctx context.Context, mediaURL, accessToken string) (io.ReadCloser, error)
}

type TokenCipher interface {
	Decrypt(ciphertext []byte) (string, error)
}

type StorageUploader interface {
	Upload(ctx context.Context, teamID uuid.UUID, namespace, filename, mimeType string, body io.Reader) (*storageclient.StoredObject, error)
}

func NewMessageHandler(
	resolver *resolver.TenantResolver,
	channels channel.Repository,
	contacts *contact.Service,
	conversations *conversation.Service,
	messages *message.Service,
	inbox Syncer,
	metaMedia MetaMediaClient,
	tokenCipher TokenCipher,
	storage StorageUploader,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *MessageHandler {
	if log == nil {
		log = slog.Default()
	}
	return &MessageHandler{
		resolver:      resolver,
		channels:      channels,
		contacts:      contacts,
		conversations: conversations,
		messages:      messages,
		inbox:         inbox,
		metaMedia:     metaMedia,
		tokenCipher:   tokenCipher,
		storage:       storage,
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
	attachment, attachErr := h.syncInboundAttachment(ctx, tenant, payload)
	if attachErr != nil {
		if h.log == nil {
			h.log = slog.Default()
		}
		h.log.Warn("inbound media sync failed",
			"event_id", event.EventID,
			"wamid", payload.WAMID,
			"message_type", payload.MessageType,
			"error", attachErr,
		)
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
	if msgCreated && attachment != nil {
		if err := h.messages.AttachToMessage(ctx, tenant.TeamID, msg.ID, attachment); err != nil {
			return err
		}
		msg.Attachment = attachment
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

func (h *MessageHandler) syncInboundAttachment(ctx context.Context, tenant *resolver.TenantContext, payload sharedevents.MetaMessagePayload) (*message.Attachment, error) {
	mediaID, fallbackFilename := extractInboundMediaRef(payload.MessageType, payload.Raw)
	if strings.TrimSpace(mediaID) == "" {
		return nil, nil
	}
	if h.channels == nil || h.metaMedia == nil || h.tokenCipher == nil || h.storage == nil {
		return nil, fmt.Errorf("inbound media dependencies are not configured")
	}
	creds, err := h.channels.FindCredentials(ctx, tenant.ChannelID)
	if err != nil {
		return nil, err
	}
	token, err := h.tokenCipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return nil, err
	}
	metaInfo, err := h.metaMedia.GetMediaMetadata(ctx, mediaID, payload.PhoneNumberID, token)
	if err != nil {
		return nil, err
	}
	reader, err := h.metaMedia.DownloadMedia(ctx, metaInfo.URL, token)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	mimeType := normalizeInboundMIME(metaInfo.MimeType, payload.MessageType)
	filename := buildInboundFilename(fallbackFilename, metaInfo.ID, mimeType)
	uploaded, err := h.storage.Upload(ctx, tenant.TeamID, "attachments", filename, mimeType, reader)
	if err != nil {
		return nil, err
	}
	return &message.Attachment{
		StorageObjectID: uploaded.ID,
		MimeType:        uploaded.MimeType,
		SizeBytes:       uploaded.SizeBytes,
		Filename:        filename,
	}, nil
}

func extractInboundMediaRef(messageType string, raw json.RawMessage) (mediaID string, filename string) {
	var media struct {
		Image    *struct{ ID string `json:"id"` } `json:"image"`
		Video    *struct{ ID string `json:"id"` } `json:"video"`
		Audio    *struct{ ID string `json:"id"` } `json:"audio"`
		Document *struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"document"`
		Sticker *struct{ ID string `json:"id"` } `json:"sticker"`
	}
	if err := json.Unmarshal(raw, &media); err != nil {
		return "", ""
	}
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "image":
		if media.Image != nil {
			return strings.TrimSpace(media.Image.ID), ""
		}
	case "video":
		if media.Video != nil {
			return strings.TrimSpace(media.Video.ID), ""
		}
	case "audio":
		if media.Audio != nil {
			return strings.TrimSpace(media.Audio.ID), ""
		}
	case "document":
		if media.Document != nil {
			return strings.TrimSpace(media.Document.ID), strings.TrimSpace(media.Document.Filename)
		}
	case "sticker":
		if media.Sticker != nil {
			return strings.TrimSpace(media.Sticker.ID), ""
		}
	}
	return "", ""
}

func normalizeInboundMIME(metaMIME, messageType string) string {
	metaMIME = strings.ToLower(strings.TrimSpace(metaMIME))
	if metaMIME != "" && metaMIME != "application/octet-stream" {
		return metaMIME
	}
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "image":
		return "image/jpeg"
	case "sticker":
		return "image/webp"
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/ogg"
	case "document":
		return "application/pdf"
	default:
		return metaMIME
	}
}

func buildInboundFilename(fallbackFilename, mediaID, mimeType string) string {
	if name := strings.TrimSpace(fallbackFilename); name != "" {
		return filepath.Base(name)
	}
	base := strings.TrimSpace(mediaID)
	if base == "" {
		base = "attachment"
	}
	ext := ""
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
	case "video/3gpp":
		ext = ".3gp"
	case "audio/aac":
		ext = ".aac"
	case "audio/amr":
		ext = ".amr"
	case "audio/mpeg":
		ext = ".mp3"
	case "audio/mp4":
		ext = ".m4a"
	case "audio/ogg":
		ext = ".ogg"
	case "application/pdf":
		ext = ".pdf"
	case "application/msword":
		ext = ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		ext = ".docx"
	case "application/vnd.ms-powerpoint":
		ext = ".ppt"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		ext = ".pptx"
	case "application/vnd.ms-excel":
		ext = ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		ext = ".xlsx"
	}
	if ext != "" {
		return base + ext
	}
	return base
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
	channelSync CoexSyncHandler
	idempotency idempotency.Store
	metrics     *Metrics
	log         *slog.Logger
}

type CoexSyncHandler interface {
	HandleCoexWebhook(ctx context.Context, events *metaclient.CoexWebhookEvents) error
}

func NewChannelHandler(
	resolver *resolver.TenantResolver,
	channels channel.Repository,
	channelSync CoexSyncHandler,
	store idempotency.Store,
	metrics *Metrics,
	log *slog.Logger,
) *ChannelHandler {
	if log == nil {
		log = slog.Default()
	}
	return &ChannelHandler{resolver: resolver, channels: channels, channelSync: channelSync, idempotency: store, metrics: metrics, log: log}
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
	if h.channelSync != nil && isCoexSyncField(payload.Field) && len(payload.Raw) > 0 {
		wrapped := fmt.Sprintf(
			`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":%q,"value":%s}]}]}`,
			payload.Field,
			string(payload.Raw),
		)
		events, err := metaclient.ParseCoexWebhookPayload([]byte(wrapped))
		if err != nil {
			return fmt.Errorf("parse coex channel update: %w", err)
		}
		return h.channelSync.HandleCoexWebhook(ctx, events)
	}
	return nil
}

func isCoexSyncField(field string) bool {
	switch field {
	case "smb_app_state_sync", "history", "smb_message_echoes":
		return true
	default:
		return false
	}
}
