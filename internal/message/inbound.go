package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// IsInstagramOGShareType reports whether the Meta attachment should be shown
// as an Open Graph-style link card instead of downloaded media.
func IsInstagramOGShareType(messageType string) bool {
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "ig_post", "post", "ig_reel", "reel", "share", "fallback":
		return true
	default:
		return false
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
	case "story_mention", "story_reply", "ig_reel", "reel", "share", "ig_post", "post", "ig_story", "fallback":
		return ContentOther
	default:
		return ContentOther
	}
}

func formatInstagramShareContent(label, title, mediaURL string) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(label) != "" {
		parts = append(parts, strings.TrimSpace(label))
	}
	if t := strings.TrimSpace(title); t != "" {
		parts = append(parts, t)
	}
	if u := strings.TrimSpace(mediaURL); u != "" {
		parts = append(parts, u)
	}
	return strings.Join(parts, "\n")
}

// InstagramShare describes a shared Instagram post/reel/link attachment.
type InstagramShare struct {
	Label   string
	Title   string
	URL     string
	MediaID string
	Type    string
}

// IsInstagramPublicURL reports whether url is a public Instagram permalink
// (e.g. /p/, /reel/, /share/) rather than a Meta messaging CDN URL.
func IsInstagramPublicURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "instagram.com" && host != "www.instagram.com" {
		return false
	}
	path := strings.ToLower(parsed.Path)
	return strings.Contains(path, "/p/") ||
		strings.Contains(path, "/reel") ||
		strings.Contains(path, "/share/") ||
		strings.Contains(path, "/tv/")
}

func shareAttachmentRank(attType, mediaURL string) int {
	rank := 0
	switch strings.ToLower(strings.TrimSpace(attType)) {
	case "ig_reel", "reel":
		rank = 40
	case "ig_post", "post":
		rank = 30
	case "share":
		rank = 20
	case "fallback":
		rank = 10
	default:
		return 0
	}
	if IsInstagramPublicURL(mediaURL) {
		rank += 100
	} else if strings.TrimSpace(mediaURL) != "" {
		rank += 10
	}
	return rank
}

func stringifyJSONID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return strings.TrimSpace(asNumber.String())
	}
	return ""
}

func assetIDFromMediaURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if id := strings.TrimSpace(parsed.Query().Get("asset_id")); id != "" {
		return id
	}
	return ""
}

func labelForShareType(attType string) string {
	switch strings.ToLower(strings.TrimSpace(attType)) {
	case "ig_reel", "reel":
		return "Shared a reel"
	case "ig_post", "post", "share":
		return "Shared a post"
	case "fallback":
		return "Link"
	default:
		return ""
	}
}

// ExtractInstagramShare picks the best post/reel/share/fallback attachment from a raw IG message.
// Prefers ig_post/ig_reel over legacy share, and Instagram permalinks over CDN URLs.
func ExtractInstagramShare(raw json.RawMessage) *InstagramShare {
	var payload struct {
		Attachments []struct {
			Type    string `json:"type"`
			Payload struct {
				URL           string          `json:"url"`
				Title         string          `json:"title"`
				StoryMediaURL string          `json:"story_media_url"`
				IGPostMediaID string          `json:"ig_post_media_id"`
				ID            json.RawMessage `json:"id"`
				ReelVideoID   json.RawMessage `json:"reel_video_id"`
			} `json:"payload"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	var best *InstagramShare
	bestRank := 0
	for _, attachment := range payload.Attachments {
		attType := strings.ToLower(strings.TrimSpace(attachment.Type))
		mediaURL := strings.TrimSpace(attachment.Payload.URL)
		if mediaURL == "" {
			mediaURL = strings.TrimSpace(attachment.Payload.StoryMediaURL)
		}
		rank := shareAttachmentRank(attType, mediaURL)
		if rank == 0 {
			continue
		}
		mediaID := strings.TrimSpace(attachment.Payload.IGPostMediaID)
		if mediaID == "" {
			mediaID = stringifyJSONID(attachment.Payload.ID)
		}
		if mediaID == "" {
			mediaID = stringifyJSONID(attachment.Payload.ReelVideoID)
		}
		if mediaID == "" {
			mediaID = assetIDFromMediaURL(mediaURL)
		}
		candidate := &InstagramShare{
			Label:   labelForShareType(attType),
			Title:   strings.TrimSpace(attachment.Payload.Title),
			URL:     mediaURL,
			MediaID: mediaID,
			Type:    attType,
		}
		if best == nil || rank > bestRank {
			best = candidate
			bestRank = rank
		}
	}
	return best
}

// WithInstagramShareURL rebuilds share content using a resolved public Instagram URL.
func WithInstagramShareURL(share *InstagramShare, publicURL string) string {
	if share == nil {
		return ""
	}
	return formatInstagramShareContent(share.Label, share.Title, publicURL)
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

	var storyAttachment struct {
		Attachments []struct {
			Type string `json:"type"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &storyAttachment); err == nil {
		for _, attachment := range storyAttachment.Attachments {
			switch strings.ToLower(strings.TrimSpace(attachment.Type)) {
			case "story_mention":
				return "Mentioned your story"
			case "ig_story":
				return "Shared a story"
			}
		}
	}

	if share := ExtractInstagramShare(raw); share != nil {
		return formatInstagramShareContent(share.Label, share.Title, share.URL)
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
	case "ig_post", "post", "share":
		return "Shared a post"
	case "fallback":
		return "Link"
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
