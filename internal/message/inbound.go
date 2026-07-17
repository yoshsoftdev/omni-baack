package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// InboundInput is system-level input for webhook consumer ingestion (no RBAC).
type InboundInput struct {
	TeamID         uuid.UUID
	ChannelID      uuid.UUID
	ConversationID uuid.UUID
	WAMID          string
	ReplyToWAMID   string
	Sender         string
	Recipient      *string
	Content        string
	ContentType    ContentType
	RawPayload     json.RawMessage
	ReceivedAt     time.Time
}

// IngestInbound stores an inbound message idempotently by wamid.
func (s *Service) IngestInbound(ctx context.Context, input InboundInput) (*Message, bool, error) {
	wamid := strings.TrimSpace(input.WAMID)
	if wamid == "" {
		return nil, false, errors.New("wamid is required")
	}
	if existing, err := s.messages.FindByWAMID(ctx, wamid); err == nil && existing != nil {
		return existing, false, nil
	} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = previewForType(input.ContentType)
	}
	sender := strings.TrimSpace(input.Sender)
	if sender == "" {
		return nil, false, errors.New("sender is required")
	}
	contentType := input.ContentType
	if contentType == "" {
		contentType = ContentText
	}

	now := time.Now().UTC()
	receivedAt := input.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = now
	}
	channelID := input.ChannelID
	wamidCopy := wamid
	replyToWAMID := strings.TrimSpace(input.ReplyToWAMID)
	replyToMessageID := s.resolveReplyTarget(ctx, input.TeamID, input.ConversationID, replyToWAMID)
	m := &Message{
		ID:             uuid.New(),
		ConversationID: input.ConversationID,
		TeamID:         input.TeamID,
		ChannelID:      &channelID,
		Direction:      DirectionInbound,
		Sender:         sender,
		Recipient:      input.Recipient,
		Content:        content,
		ContentType:    contentType,
		WAMID:          &wamidCopy,
		ReplyToWAMID:   nullableString(replyToWAMID),
		ReplyToMessageID: replyToMessageID,
		RawPayload:     input.RawPayload,
		ReceivedAt:     receivedAt,
		CreatedAt:      now,
	}
	if err := s.messages.Create(ctx, m); err != nil {
		if existing, findErr := s.messages.FindByWAMID(ctx, wamid); findErr == nil && existing != nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	return m, true, nil
}

// OutboundEchoInput is system-level input for provider echo webhooks (IG app / WA Business app).
type OutboundEchoInput struct {
	TeamID         uuid.UUID
	ChannelID      uuid.UUID
	ConversationID uuid.UUID
	WAMID          string
	ReplyToWAMID   string
	Sender         string
	Recipient      string
	Content        string
	ContentType    ContentType
	RawPayload     json.RawMessage
	ReceivedAt     time.Time
}

// IngestOutboundEcho stores an outbound echo idempotently by wamid (e.g. Instagram is_echo).
func (s *Service) IngestOutboundEcho(ctx context.Context, input OutboundEchoInput) (*Message, bool, error) {
	wamid := strings.TrimSpace(input.WAMID)
	if wamid == "" {
		return nil, false, errors.New("wamid is required")
	}
	if existing, err := s.messages.FindByWAMID(ctx, wamid); err == nil && existing != nil {
		return existing, false, nil
	} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = previewForType(input.ContentType)
	}
	sender := strings.TrimSpace(input.Sender)
	if sender == "" {
		sender = "instagram_app"
	}
	contentType := input.ContentType
	if contentType == "" {
		contentType = ContentText
	}

	now := time.Now().UTC()
	receivedAt := input.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = now
	}
	channelID := input.ChannelID
	wamidCopy := wamid
	replyToWAMID := strings.TrimSpace(input.ReplyToWAMID)
	replyToMessageID := s.resolveReplyTarget(ctx, input.TeamID, input.ConversationID, replyToWAMID)
	sentStatus := StatusSent
	m := &Message{
		ID:               uuid.New(),
		ConversationID:   input.ConversationID,
		TeamID:           input.TeamID,
		ChannelID:        &channelID,
		Direction:        DirectionOutbound,
		Sender:           sender,
		Recipient:        nullableString(input.Recipient),
		Content:          content,
		ContentType:      contentType,
		WAMID:            &wamidCopy,
		ReplyToWAMID:     nullableString(replyToWAMID),
		ReplyToMessageID: replyToMessageID,
		Status:           &sentStatus,
		RawPayload:       input.RawPayload,
		ReceivedAt:       receivedAt,
		CreatedAt:        now,
	}
	if err := s.messages.Create(ctx, m); err != nil {
		if existing, findErr := s.messages.FindByWAMID(ctx, wamid); findErr == nil && existing != nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	return m, true, nil
}

func nullableString(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	s := strings.TrimSpace(v)
	return &s
}

func (s *Service) AttachToMessage(ctx context.Context, teamID, messageID uuid.UUID, attachment *Attachment) error {
	if s.attachments == nil || attachment == nil {
		return nil
	}
	if attachment.StorageObjectID == uuid.Nil {
		return fmt.Errorf("storage object id is required")
	}
	if err := s.attachments.Create(ctx, &MessageAttachment{
		ID:              uuid.New(),
		MessageID:       messageID,
		TeamID:          teamID,
		StorageObjectID: attachment.StorageObjectID,
		MimeType:        attachment.MimeType,
		SizeBytes:       attachment.SizeBytes,
		Filename:        attachment.Filename,
	}); err != nil {
		return err
	}
	return nil
}

// ApplyDeliveryStatus updates message delivery status with monotonic progression.
func (s *Service) ApplyDeliveryStatus(ctx context.Context, wamid string, status Status) error {
	wamid = strings.TrimSpace(wamid)
	if wamid == "" {
		return errors.New("wamid is required")
	}
	m, err := s.messages.FindByWAMID(ctx, wamid)
	if err != nil {
		return err
	}
	if !shouldApplyStatus(m.Status, status) {
		return nil
	}
	if err := s.messages.UpdateStatus(ctx, m.TeamID, m.ID, status); err != nil {
		return err
	}
	s.publishStatusUpdate(ctx, m, status)
	return nil
}

func shouldApplyStatus(current *Status, next Status) bool {
	if current == nil {
		return true
	}
	if *current == next {
		return false
	}
	if next == StatusFailed {
		return *current != StatusRead
	}
	return statusRank(next) > statusRank(*current)
}

func statusRank(s Status) int {
	switch s {
	case StatusSent:
		return 1
	case StatusDelivered:
		return 2
	case StatusRead:
		return 3
	case StatusFailed:
		return 4
	default:
		return 0
	}
}

// ContentTypeFromMeta maps Meta message types to storage content types.
func ContentTypeFromMeta(messageType string) ContentType {
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "text", "attachment":
		return ContentText
	case "image":
		return ContentImage
	case "video":
		return ContentVideo
	case "audio":
		return ContentAudio
	case "document", "file":
		return ContentDocument
	case "template":
		return ContentTemplate
	case "sticker":
		return ContentImage
	case "story_mention", "story_reply", "ig_reel", "reel", "share", "ig_post", "ig_story":
		return ContentOther
	default:
		return ContentOther
	}
}

// ExtractTextContent returns a display string from a Meta raw message payload.
func ExtractTextContent(messageType string, raw json.RawMessage) string {
	var instagramText struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &instagramText); err == nil && strings.TrimSpace(instagramText.Text) != "" {
		return instagramText.Text
	}
	var instagramReply struct {
		ReplyTo *struct {
			Story *struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"story"`
			MID string `json:"mid"`
		} `json:"reply_to"`
	}
	if err := json.Unmarshal(raw, &instagramReply); err == nil && instagramReply.ReplyTo != nil && instagramReply.ReplyTo.Story != nil {
		return "Replied to a story"
	}
	var instagramAttachment struct {
		Attachments []struct {
			Type    string `json:"type"`
			Payload struct {
				URL           string `json:"url"`
				Title         string `json:"title"`
				StoryMediaURL string `json:"story_media_url"`
			} `json:"payload"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &instagramAttachment); err == nil {
		for _, attachment := range instagramAttachment.Attachments {
			switch strings.ToLower(strings.TrimSpace(attachment.Type)) {
			case "story_mention":
				return "Mentioned your story"
			case "ig_story":
				return "Shared a story"
			case "ig_reel", "reel":
				return "Shared a reel"
			case "ig_post", "share":
				if title := strings.TrimSpace(attachment.Payload.Title); title != "" {
					return "Shared a post\n" + title
				}
				return "Shared a post"
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "text":
		var body struct {
			Text struct {
				Body string `json:"body"`
			} `json:"text"`
		}
		if err := json.Unmarshal(raw, &body); err == nil && body.Text.Body != "" {
			return body.Text.Body
		}
	case "image", "video", "audio", "document", "file", "sticker":
		var media struct {
			Image    *struct{ Caption string `json:"caption"` } `json:"image"`
			Video    *struct{ Caption string `json:"caption"` } `json:"video"`
			Audio    *struct{} `json:"audio"`
			Document *struct{ Caption string `json:"caption"`; Filename string `json:"filename"` } `json:"document"`
			Sticker  *struct{} `json:"sticker"`
			File     *struct{ Url string `json:"url"` } `json:"file"`
		}
		if err := json.Unmarshal(raw, &media); err == nil {
			switch messageType {
			case "image":
				if media.Image != nil && media.Image.Caption != "" {
					return media.Image.Caption
				}
			case "video":
				if media.Video != nil && media.Video.Caption != "" {
					return media.Video.Caption
				}
			case "document":
				if media.Document != nil {
					if media.Document.Caption != "" {
						return media.Document.Caption
					}
					return media.Document.Filename
				}
			case "file":
				if media.File != nil {
					return "File"
				}
			}
		}
	case "location":
		var loc struct {
			Location struct {
				Name    string `json:"name"`
				Address string `json:"address"`
			} `json:"location"`
		}
		if err := json.Unmarshal(raw, &loc); err == nil {
			if loc.Location.Name != "" {
				return loc.Location.Name
			}
			return loc.Location.Address
		}
	case "contacts":
		return "Contact shared"
	case "story_mention":
		return "Mentioned your story"
	case "story_reply":
		return "Replied to a story"
	case "ig_story":
		return "Shared a story"
	case "ig_reel", "reel":
		return "Shared a reel"
	case "ig_post", "share":
		return "Shared a post"
	}
	return previewForType(ContentTypeFromMeta(messageType))
}

func previewForType(ct ContentType) string {
	switch ct {
	case ContentImage:
		return "Image"
	case ContentVideo:
		return "Video"
	case ContentAudio:
		return "Audio"
	case ContentDocument:
		return "Document"
	case ContentTemplate:
		return "Template message"
	default:
		return "Message"
	}
}

// ParseMetaTimestamp parses Meta unix timestamp strings.
func ParseMetaTimestamp(raw string) time.Time {
	if raw == "" {
		return time.Now().UTC()
	}
	if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(ts, 0).UTC()
	}
	return time.Now().UTC()
}

// DecodeMetaMessagePayload decodes canonical message payload.
func DecodeMetaMessagePayload(payload []byte) (sharedevents.MetaMessagePayload, error) {
	var p sharedevents.MetaMessagePayload
	err := json.Unmarshal(payload, &p)
	return p, err
}

// DecodeMetaStatusPayload decodes canonical status payload.
func DecodeMetaStatusPayload(payload []byte) (sharedevents.MetaStatusPayload, error) {
	var p sharedevents.MetaStatusPayload
	err := json.Unmarshal(payload, &p)
	return p, err
}
