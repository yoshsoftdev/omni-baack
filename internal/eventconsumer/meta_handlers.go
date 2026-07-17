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
	"github.com/omnichannel/omni-chan-v2/internal/debug"
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
	HandleOutbound(ctx context.Context, p inbox.OutboundParams) (*conversation.Conversation, error)
}

type MessageHandler struct {
	resolver          *resolver.TenantResolver
	channels          channel.Repository
	contacts          *contact.Service
	conversations     *conversation.Service
	messages          *message.Service
	inbox             Syncer
	metaMedia         MetaMediaClient
	instagramProfile  InstagramProfileClient
	tokenCipher       TokenCipher
	storage           StorageUploader
	idempotency       idempotency.Store
	metrics           *Metrics
	log               *slog.Logger
}

type MetaMediaClient interface {
	GetMediaMetadata(ctx context.Context, mediaID, phoneNumberID, accessToken string) (*metaclient.MediaMetadata, error)
	DownloadMedia(ctx context.Context, mediaURL, accessToken string) (io.ReadCloser, error)
}

type InstagramProfileClient interface {
	GetInstagramScopedUser(ctx context.Context, scopedUserID, accessToken string) (*metaclient.InstagramScopedUser, error)
	GetInstagramMediaPermalink(ctx context.Context, mediaID, accessToken string) (string, error)
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
	instagramProfile InstagramProfileClient,
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
		resolver:         resolver,
		channels:         channels,
		contacts:         contacts,
		conversations:    conversations,
		messages:         messages,
		inbox:            inbox,
		metaMedia:        metaMedia,
		instagramProfile: instagramProfile,
		tokenCipher:      tokenCipher,
		storage:          storage,
		idempotency:      store,
		metrics:          metrics,
		log:              log,
	}
}

func (h *MessageHandler) Handle(ctx context.Context, event sharedevents.CanonicalEvent) error {
	payload, err := message.DecodeMetaMessagePayload(event.Payload)
	if err != nil || payload.WAMID == "" {
		return TerminalDLQ("message", ErrMalformedEvent)
	}

	if isInstagramInbound(payload) {
		return h.handleInstagram(ctx, event, payload)
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

func isInstagramInbound(payload sharedevents.MetaMessagePayload) bool {
	return strings.EqualFold(strings.TrimSpace(payload.Platform), "instagram") ||
		strings.TrimSpace(payload.IGUserID) != ""
}

func (h *MessageHandler) handleInstagram(ctx context.Context, event sharedevents.CanonicalEvent, payload sharedevents.MetaMessagePayload) error {
	tenant, err := h.resolver.ResolveByIGUserID(ctx, payload.IGUserID)
	if err != nil {
		return err
	}

	key := idempotency.MessageKey("instagram", payload.WAMID)
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		return h.processInstagram(ctx, event, tenant, payload)
	})
	if err != nil {
		return err
	}
	if skipped {
		h.log.Info("duplicate instagram message skipped", "event_id", event.EventID, "mid", payload.WAMID)
	}
	return nil
}

func (h *MessageHandler) processInstagram(ctx context.Context, event sharedevents.CanonicalEvent, tenant *resolver.TenantContext, payload sharedevents.MetaMessagePayload) error {
	if payload.IsEcho {
		return h.processInstagramEcho(ctx, event, tenant, payload)
	}

	senderID := strings.TrimSpace(payload.From)
	if senderID == "" {
		senderID = strings.TrimSpace(payload.ContactWAID)
	}

	displayName, username, avatarURL := h.fetchInstagramSenderProfile(ctx, tenant.ChannelID, senderID)
	contactEntity, err := h.contacts.ResolveInstagramInbound(ctx, tenant.TeamID, senderID, displayName, username, avatarURL)
	if err != nil {
		return err
	}

	content := message.ExtractTextContent(payload.MessageType, payload.Raw)
	if content == "" {
		content = "New message"
	}
	if message.IsInstagramOGShareType(payload.MessageType) {
		content = h.enrichInstagramShareContent(ctx, tenant.ChannelID, payload.Raw, content)
	}
	contentType := message.ContentTypeFromMeta(payload.MessageType)
	receivedAt := message.ParseMetaTimestamp(payload.Timestamp)
	if !event.OccurredAt.IsZero() {
		receivedAt = event.OccurredAt
	}

	var attachment *message.Attachment
	if !message.IsInstagramOGShareType(payload.MessageType) {
		var attachErr error
		attachment, attachErr = h.syncInboundAttachment(ctx, tenant, payload)
		if attachErr != nil {
			if h.log == nil {
				h.log = slog.Default()
			}
			h.log.Warn("instagram inbound media sync failed",
				"event_id", event.EventID,
				"mid", payload.WAMID,
				"message_type", payload.MessageType,
				"error", attachErr,
			)
		}
		if attachment != nil {
			contentType = preferAttachmentContentType(contentType, attachment.MimeType, payload.MessageType)
		}
	}

	conv, convCreated, err := h.conversations.FindOrCreateForInbound(ctx, tenant.TeamID, contactEntity.ID, tenant.ChannelID, conversation.ChannelInstagram)
	if err != nil {
		return err
	}
	if convCreated {
		h.metrics.IncConversationsCreated()
	}

	msg, msgCreated, err := h.messages.IngestInbound(ctx, message.InboundInput{
		TeamID:         tenant.TeamID,
		ChannelID:      tenant.ChannelID,
		ConversationID: conv.ID,
		WAMID:          payload.WAMID,
		ReplyToWAMID:   payload.ReplyToWAMID,
		Sender:         senderID,
		Content:        content,
		ContentType:    contentType,
		RawPayload:     payload.Raw,
		ReceivedAt:     receivedAt,
	})
	if err != nil {
		h.log.Error("instagram message ingest failed",
			"event_id", event.EventID,
			"conversation_id", conv.ID.String(),
			"mid_len", len(strings.TrimSpace(payload.WAMID)),
			"error", err.Error(),
		)
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
	h.log.Info("instagram inbound message ingested",
		"event_id", event.EventID,
		"team_id", tenant.TeamID.String(),
		"channel_id", tenant.ChannelID.String(),
		"conversation_id", conv.ID.String(),
		"message_id", msg.ID.String(),
		"contact", contactEntity.ContactName,
		"has_attachment", attachment != nil,
	)
	return nil
}

func (h *MessageHandler) processInstagramEcho(ctx context.Context, event sharedevents.CanonicalEvent, tenant *resolver.TenantContext, payload sharedevents.MetaMessagePayload) error {
	contactID := strings.TrimSpace(payload.To)
	if contactID == "" {
		contactID = strings.TrimSpace(payload.ContactWAID)
	}
	if contactID == "" {
		return TerminalDLQ("message", errors.New("instagram echo missing recipient"))
	}

	displayName, username, avatarURL := h.fetchInstagramSenderProfile(ctx, tenant.ChannelID, contactID)
	contactEntity, err := h.contacts.ResolveInstagramInbound(ctx, tenant.TeamID, contactID, displayName, username, avatarURL)
	if err != nil {
		return err
	}

	content := message.ExtractTextContent(payload.MessageType, payload.Raw)
	if content == "" {
		content = "New message"
	}
	if message.IsInstagramOGShareType(payload.MessageType) {
		content = h.enrichInstagramShareContent(ctx, tenant.ChannelID, payload.Raw, content)
	}
	contentType := message.ContentTypeFromMeta(payload.MessageType)
	receivedAt := message.ParseMetaTimestamp(payload.Timestamp)
	if !event.OccurredAt.IsZero() {
		receivedAt = event.OccurredAt
	}

	var attachment *message.Attachment
	if !message.IsInstagramOGShareType(payload.MessageType) {
		var attachErr error
		attachment, attachErr = h.syncInboundAttachment(ctx, tenant, payload)
		if attachErr != nil {
			if h.log == nil {
				h.log = slog.Default()
			}
			h.log.Warn("instagram echo media sync failed",
				"event_id", event.EventID,
				"mid", payload.WAMID,
				"message_type", payload.MessageType,
				"error", attachErr,
			)
		}
		if attachment != nil {
			contentType = preferAttachmentContentType(contentType, attachment.MimeType, payload.MessageType)
		}
	}

	conv, convCreated, err := h.conversations.FindOrCreateForInbound(ctx, tenant.TeamID, contactEntity.ID, tenant.ChannelID, conversation.ChannelInstagram)
	if err != nil {
		return err
	}
	if convCreated {
		h.metrics.IncConversationsCreated()
	}

	businessSender := strings.TrimSpace(payload.From)
	if businessSender == "" {
		businessSender = "instagram_app"
	}
	msg, msgCreated, err := h.messages.IngestOutboundEcho(ctx, message.OutboundEchoInput{
		TeamID:         tenant.TeamID,
		ChannelID:      tenant.ChannelID,
		ConversationID: conv.ID,
		WAMID:          payload.WAMID,
		ReplyToWAMID:   payload.ReplyToWAMID,
		Sender:         businessSender,
		Recipient:      contactEntity.Phone,
		Content:        content,
		ContentType:    contentType,
		RawPayload:     payload.Raw,
		ReceivedAt:     receivedAt,
	})
	if err != nil {
		h.log.Error("instagram echo ingest failed",
			"event_id", event.EventID,
			"conversation_id", conv.ID.String(),
			"mid_len", len(strings.TrimSpace(payload.WAMID)),
			"error", err.Error(),
		)
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
	if msgCreated && h.inbox != nil {
		if _, err := h.inbox.HandleOutbound(ctx, inbox.OutboundParams{
			TeamID:       tenant.TeamID,
			AgentID:      uuid.Nil,
			Conversation: conv,
			Message:      msg,
		}); err != nil {
			return err
		}
	}

	if h.log == nil {
		h.log = slog.Default()
	}
	h.log.Info("instagram echo message ingested",
		"event_id", event.EventID,
		"team_id", tenant.TeamID.String(),
		"channel_id", tenant.ChannelID.String(),
		"conversation_id", conv.ID.String(),
		"message_id", msg.ID.String(),
		"contact", contactEntity.ContactName,
	)
	return nil
}

func (h *MessageHandler) fetchInstagramSenderProfile(ctx context.Context, channelID uuid.UUID, senderID string) (displayName, username, avatarURL string) {
	if h.instagramProfile == nil || h.tokenCipher == nil || h.channels == nil {
		return "", "", ""
	}
	creds, err := h.channels.FindCredentials(ctx, channelID)
	if err != nil || creds == nil {
		return "", "", ""
	}
	token, err := h.tokenCipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", "", ""
	}
	profile, err := h.instagramProfile.GetInstagramScopedUser(ctx, senderID, token)
	if err != nil {
		if h.log != nil {
			h.log.Warn("instagram sender profile lookup failed",
				"sender_id", senderID,
				"channel_id", channelID.String(),
				"error", err.Error(),
			)
		}
		return "", "", ""
	}
	return strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Username), strings.TrimSpace(profile.ProfilePic)
}

func (h *MessageHandler) enrichInstagramShareContent(ctx context.Context, channelID uuid.UUID, raw json.RawMessage, content string) string {
	share := message.ExtractInstagramShare(raw)
	if share == nil {
		return content
	}
	if message.IsInstagramPublicURL(share.URL) {
		return message.WithInstagramShareURL(share, share.URL)
	}
	if strings.TrimSpace(share.MediaID) == "" || h.instagramProfile == nil || h.tokenCipher == nil || h.channels == nil {
		return content
	}
	creds, err := h.channels.FindCredentials(ctx, channelID)
	if err != nil || creds == nil {
		return content
	}
	token, err := h.tokenCipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		return content
	}
	permalink, err := h.instagramProfile.GetInstagramMediaPermalink(ctx, share.MediaID, token)
	if err != nil || !message.IsInstagramPublicURL(permalink) {
		if err != nil && h.log != nil {
			h.log.Warn("instagram share permalink resolve failed",
				"channel_id", channelID.String(),
				"media_id", share.MediaID,
				"error", err.Error(),
			)
		}
		return content
	}
	return message.WithInstagramShareURL(share, permalink)
}

func (h *MessageHandler) process(ctx context.Context, event sharedevents.CanonicalEvent, tenant *resolver.TenantContext, payload sharedevents.MetaMessagePayload) error {
	phone := payload.ContactWAID
	if phone == "" {
		phone = payload.From
	}
	contactEntity, err := h.contacts.ResolveInbound(ctx, contact.ResolveInboundInput{
		TeamID:      tenant.TeamID,
		RawPhone:    phone,
		ProfileName: payload.ContactName,
		Channel:     "whatsapp",
		ExternalID:  payload.ContactWAID,
		Source:      contact.SourceMetaInbound,
	})
	if err != nil {
		return err
	}

	conv, convCreated, err := h.conversations.FindOrCreateForInbound(ctx, tenant.TeamID, contactEntity.ID, tenant.ChannelID, conversation.ChannelWhatsApp)
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
		ReplyToWAMID:   payload.ReplyToWAMID,
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

	mediaID, fallbackFilename := extractInboundMediaRef(payload.MessageType, payload.Raw)
	if strings.TrimSpace(mediaID) != "" {
		metaInfo, metaErr := h.metaMedia.GetMediaMetadata(ctx, mediaID, payload.PhoneNumberID, token)
		if metaErr != nil {
			return nil, metaErr
		}
		reader, dlErr := h.metaMedia.DownloadMedia(ctx, metaInfo.URL, token)
		if dlErr != nil {
			return nil, dlErr
		}
		defer reader.Close()
		mimeType := normalizeInboundMIME(metaInfo.MimeType, payload.MessageType)
		filename := buildInboundFilename(fallbackFilename, metaInfo.ID, mimeType)
		uploaded, upErr := h.storage.Upload(ctx, tenant.TeamID, "attachments", filename, mimeType, reader)
		if upErr != nil {
			return nil, upErr
		}
		return &message.Attachment{
			StorageObjectID: uploaded.ID,
			MimeType:        uploaded.MimeType,
			SizeBytes:       uploaded.SizeBytes,
			Filename:        filename,
		}, nil
	}

	mediaURL, attType, igFilename := extractInstagramAttachmentURL(payload.Raw)
	if strings.TrimSpace(mediaURL) == "" {
		return nil, nil
	}
	if !metaclient.IsAllowedMetaMediaURL(mediaURL) {
		return nil, fmt.Errorf("instagram media url host is not allowed")
	}
	reader, dlErr := h.metaMedia.DownloadMedia(ctx, mediaURL, token)
	if dlErr != nil {
		return nil, dlErr
	}
	defer reader.Close()

	messageType := payload.MessageType
	if strings.TrimSpace(messageType) == "" {
		messageType = attType
	}
	mimeType := normalizeInboundMIME("", messageType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mimeHintFromInstagramAttachment(attType, mediaURL)
	}
	filename := buildInboundFilename(igFilename, "ig-media", mimeType)
	uploaded, upErr := h.storage.Upload(ctx, tenant.TeamID, "attachments", filename, mimeType, reader)
	if upErr != nil {
		return nil, upErr
	}
	return &message.Attachment{
		StorageObjectID: uploaded.ID,
		MimeType:        uploaded.MimeType,
		SizeBytes:       uploaded.SizeBytes,
		Filename:        filename,
	}, nil
}

func extractInstagramAttachmentURL(raw json.RawMessage) (mediaURL, attachmentType, filename string) {
	var payload struct {
		Attachments []struct {
			Type    string `json:"type"`
			Payload struct {
				URL           string `json:"url"`
				Title         string `json:"title"`
				StoryMediaURL string `json:"story_media_url"`
			} `json:"payload"`
		} `json:"attachments"`
		ReplyTo *struct {
			Story *struct {
				URL string `json:"url"`
			} `json:"story"`
		} `json:"reply_to"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", ""
	}
	for _, attachment := range payload.Attachments {
		attType := strings.ToLower(strings.TrimSpace(attachment.Type))
		url := strings.TrimSpace(attachment.Payload.URL)
		if url == "" {
			url = strings.TrimSpace(attachment.Payload.StoryMediaURL)
		}
		if url == "" {
			continue
		}
		name := ""
		if title := strings.TrimSpace(attachment.Payload.Title); title != "" {
			name = sanitizeAttachmentFilename(title)
		}
		return url, attType, name
	}
	if payload.ReplyTo != nil && payload.ReplyTo.Story != nil {
		if url := strings.TrimSpace(payload.ReplyTo.Story.URL); url != "" {
			return url, "story_reply", ""
		}
	}
	return "", "", ""
}

func mimeHintFromInstagramAttachment(attType, mediaURL string) string {
	lowerURL := strings.ToLower(mediaURL)
	switch {
	case strings.Contains(lowerURL, ".mp4"), strings.Contains(lowerURL, "video"):
		return "video/mp4"
	case strings.Contains(lowerURL, ".png"):
		return "image/png"
	case strings.Contains(lowerURL, ".webp"):
		return "image/webp"
	case strings.Contains(lowerURL, ".pdf"):
		return "application/pdf"
	}
	switch strings.ToLower(strings.TrimSpace(attType)) {
	case "video", "ig_reel", "reel":
		return "video/mp4"
	case "audio":
		return "audio/mp4"
	case "file", "document":
		return "application/pdf"
	default:
		return "image/jpeg"
	}
}

func preferAttachmentContentType(current message.ContentType, mimeType, messageType string) message.ContentType {
	if message.IsInstagramOGShareType(messageType) {
		return current
	}
	if current == message.ContentImage || current == message.ContentVideo || current == message.ContentAudio || current == message.ContentDocument {
		return current
	}
	if ct, err := message.ContentTypeFromMIME(mimeType); err == nil {
		return ct
	}
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "image", "story_mention", "ig_story", "sticker":
		return message.ContentImage
	case "video":
		return message.ContentVideo
	case "audio":
		return message.ContentAudio
	case "file", "document":
		return message.ContentDocument
	default:
		lower := strings.ToLower(mimeType)
		switch {
		case strings.HasPrefix(lower, "image/"):
			return message.ContentImage
		case strings.HasPrefix(lower, "video/"):
			return message.ContentVideo
		case strings.HasPrefix(lower, "audio/"):
			return message.ContentAudio
		default:
			return current
		}
	}
}

func sanitizeAttachmentFilename(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	title = strings.ReplaceAll(title, "\n", " ")
	runes := []rune(title)
	if len(runes) > 40 {
		title = string(runes[:40])
	}
	return strings.TrimSpace(title) + ".jpg"
}

func extractInboundMediaRef(messageType string, raw json.RawMessage) (mediaID string, filename string) {
	var media struct {
		Image *struct {
			ID string `json:"id"`
		} `json:"image"`
		Video *struct {
			ID string `json:"id"`
		} `json:"video"`
		Audio *struct {
			ID string `json:"id"`
		} `json:"audio"`
		Document *struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"document"`
		Sticker *struct {
			ID string `json:"id"`
		} `json:"sticker"`
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
	case "image", "story_mention", "ig_story", "ig_post", "share", "sticker", "story_reply":
		return "image/jpeg"
	case "video", "ig_reel", "reel":
		return "video/mp4"
	case "audio":
		return "audio/ogg"
	case "document", "file":
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
	if status == "" && event.Type != sharedevents.TypeMessageDeleted {
		return TerminalDLQ("status", ErrMalformedEvent)
	}

	statusKey := string(status)
	if event.Type == sharedevents.TypeMessageDeleted {
		statusKey = "deleted"
	}
	key := idempotency.StatusKey(event.Provider, payload.WAMID, statusKey)
	skipped, err := idempotency.ProcessOnce(ctx, h.idempotency, key, func() error {
		if event.Type == sharedevents.TypeMessageDeleted {
			if err := h.messages.MarkDeletedByWAMID(ctx, tenant.TeamID, payload.WAMID, message.DeletedBySourceWAUser); err != nil {
				if !errors.Is(err, repository.ErrNotFound) {
					return err
				}
			}
		} else {
			if err := h.messages.ApplyDeliveryStatus(ctx, payload.WAMID, status); err != nil {
				if !errors.Is(err, repository.ErrNotFound) {
					return err
				}
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

	// #region agent log
	debug.AgentLog("H1", "meta_handlers.go:ChannelHandler.Handle", "channel.updated received", map[string]any{
		"event_id": event.EventID, "field": payload.Field, "phone_number_id": payload.PhoneNumberID, "waba_id": payload.WABAID,
	})
	// #endregion

	tenant, err := h.resolver.ResolveByPhoneNumberID(ctx, payload.PhoneNumberID)
	if err != nil {
		tenant, err = h.resolver.ResolveByWABAID(ctx, payload.WABAID)
		if err != nil {
			// #region agent log
			debug.AgentLog("H1", "meta_handlers.go:ChannelHandler.Handle", "tenant resolve failed", map[string]any{
				"event_id": event.EventID, "field": payload.Field, "phone_number_id": payload.PhoneNumberID, "error": err.Error(),
			})
			// #endregion
			return err
		}
	}

	// #region agent log
	debug.AgentLog("H1", "meta_handlers.go:ChannelHandler.Handle", "tenant resolved", map[string]any{
		"event_id": event.EventID, "channel_id": tenant.ChannelID.String(), "team_id": tenant.TeamID.String(),
	})
	// #endregion

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
		// #region agent log
		debug.AgentLog("H2", "meta_handlers.go:ChannelHandler.Handle", "idempotency skipped before coex sync", map[string]any{
			"event_id": event.EventID, "field": payload.Field, "phone_number_id": payload.PhoneNumberID, "idempotency_key": key,
		})
		// #endregion
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
			// #region agent log
			debug.AgentLog("H3", "meta_handlers.go:ChannelHandler.Handle", "parse coex payload failed", map[string]any{
				"event_id": event.EventID, "field": payload.Field, "error": err.Error(),
			})
			// #endregion
			return fmt.Errorf("parse coex channel update: %w", err)
		}
		// #region agent log
		debug.AgentLog("H3", "meta_handlers.go:ChannelHandler.Handle", "dispatching coex webhook", map[string]any{
			"event_id": event.EventID, "field": payload.Field,
			"history_events": len(events.HistoryEvents), "contact_events": len(events.ContactSyncEvents),
		})
		// #endregion
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
