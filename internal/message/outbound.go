package message

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	camrecipient "github.com/omnichannel/omni-chan-v2/internal/campaign/recipient"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/media"
	"github.com/omnichannel/omni-chan-v2/internal/outbound"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// StorageDownloader fetches uploaded objects from storage-service.
type StorageDownloader interface {
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *storageclient.StoredObject, error)
	GetMetadata(ctx context.Context, teamID, objectID uuid.UUID) (*storageclient.StoredObject, error)
}

// MediaURLIssuer mints short-lived Main API media URLs for Meta to fetch.
type MediaURLIssuer interface {
	Issue(teamID, objectID uuid.UUID) (string, error)
}

// ChannelStore loads channel credentials for outbound delivery.
type ChannelStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*channel.Channel, error)
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*channel.Credentials, error)
}

// ContactStore loads Instagram addressing helpers for outbound delivery.
// WhatsApp phone MUST NOT be read from Contact — use WhatsAppAddressResolver.
type ContactStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*contact.Contact, error)
	ResolveInstagramScopedID(ctx context.Context, teamID, contactID uuid.UUID) (string, error)
}

// TokenDecryptor decrypts stored Meta access tokens.
type TokenDecryptor interface {
	Decrypt(ciphertext []byte) (string, error)
}

// WhatsAppAddressResolver resolves WhatsApp send identity from Canonical Provider.
// Same contract as Campaign — never Contact.Phone.
type WhatsAppAddressResolver interface {
	Resolve(ctx context.Context, teamID, contactID uuid.UUID) (*camrecipient.WhatsAppRecipient, error)
}

// TemplateResolver loads domain Templates and builds send components via Template Domain.
// Inbox/message MUST use this — never pkg/meta.BuildTemplatePayload or SQL on templates.
type TemplateResolver interface {
	GetByID(ctx context.Context, teamID, templateID uuid.UUID) (*template.Template, error)
	GetByName(ctx context.Context, teamID, channelID uuid.UUID, name, language string) (*template.Template, error)
	Build(tmpl *template.Template, vars builder.Variables) ([]builder.SendComponent, error)
	MetaSendLanguage(stored string) string
}

// OutboundDeps wires channel delivery for agent replies.
// Pipeline: WhatsAppAddresses → Templates/Builder → Media → Channel → Meta.
type OutboundDeps struct {
	Channels  ChannelStore
	Contacts  ContactStore
	Cipher    TokenDecryptor
	Storage   StorageDownloader
	MediaURLs MediaURLIssuer
	// WhatsApp is Channel.Send for WhatsApp Cloud (hides Meta APIs).
	WhatsApp outbound.Channel
	// Instagram is Channel.Send for Instagram Messaging (hides Meta APIs).
	Instagram outbound.Channel
	// Media is the shared WhatsApp Media Manager (same as Broadcast). Required for WA attachments.
	Media media.Manager
	// Templates is TemplateRepository + TemplateBuilder (same domain as Campaign). Optional until Meta template send.
	Templates TemplateResolver
	// WhatsAppAddresses resolves WA phone from Canonical Provider (same as Campaign).
	WhatsAppAddresses WhatsAppAddressResolver
}

type SendInput struct {
	Content          string
	StorageObjectID  *uuid.UUID
	ReplyToWAMID     string
	ReplyToMessageID *uuid.UUID
	ClientMessageID  *uuid.UUID
	// ProviderMediaID is an optional pre-resolved WhatsApp Cloud media id (compatibility).
	// When set with a WhatsApp attachment send, EnsureMedia may be skipped.
	ProviderMediaID string
}

// SendReply persists an agent reply and delivers it through the connected channel when configured.
func (s *Service) SendReply(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, content string) (*Message, error) {
	return s.SendMessage(ctx, teamID, userID, actorRole, conversationID, SendInput{Content: content})
}

// SendMessage sends a text or media message through WhatsApp or Instagram.
func (s *Service) SendMessage(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, input SendInput) (*Message, error) {
	if err := s.authorize(actorRole, rbac.PermConversationReply); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if input.ClientMessageID != nil {
		existing, findErr := s.messages.FindByClientMessageID(ctx, teamID, conversationID, *input.ClientMessageID)
		if findErr == nil && existing != nil {
			enriched := []Message{*existing}
			if err := s.enrichSenderNames(ctx, enriched); err != nil {
				return nil, err
			}
			return &enriched[0], nil
		}
		if findErr != nil && findErr != repository.ErrNotFound {
			return nil, serviceerrors.MapRepository(findErr)
		}
	}

	content := strings.TrimSpace(input.Content)
	if content == "" && input.StorageObjectID == nil {
		return nil, serviceerrors.NewValidation("content", "is required when no attachment is provided")
	}

	channelHint := conversation.ChannelWhatsApp
	if conv.ChannelType == conversation.ChannelInstagram {
		channelHint = conversation.ChannelInstagram
	}

	var attachment *Attachment
	contentType := ContentText
	if input.StorageObjectID != nil {
		if s.outbound == nil || s.outbound.Storage == nil {
			return nil, serviceerrors.NewValidation("storage_object_id", "storage service is unavailable")
		}
		objectMeta, metaErr := s.outbound.Storage.GetMetadata(ctx, teamID, *input.StorageObjectID)
		if metaErr != nil {
			return nil, serviceerrors.NewValidation("storage_object_id", "invalid storage object")
		}
		validatedType, validateErr := ValidateAttachmentForChannel(objectMeta.MimeType, objectMeta.SizeBytes, channelHint)
		if validateErr != nil {
			return nil, serviceerrors.NewValidation("attachment", validateErr.Error())
		}
		contentType = validatedType
		attachment = &Attachment{
			StorageObjectID: objectMeta.ID,
			MimeType:        objectMeta.MimeType,
			SizeBytes:       objectMeta.SizeBytes,
			Filename:        filenameFromObject(objectMeta),
		}
		if content == "" {
			content = attachment.Filename
		}
	}

	var recipient *string
	var wamid *string
	var deliveryNotice string
	replyToMID := s.resolveOutboundReplyMID(ctx, teamID, conversationID, input)

	if s.outbound != nil &&
		s.outbound.Channels != nil &&
		s.outbound.Cipher != nil &&
		conv.ChannelID != nil {
		ch, channelErr := s.outbound.Channels.FindByID(ctx, teamID, *conv.ChannelID)
		if channelErr != nil {
			return nil, serviceerrors.MapRepository(channelErr)
		}
		if ch.Status != channel.StatusConnected {
			return nil, serviceerrors.NewValidation("channel", "channel is not connected")
		}

		creds, credErr := s.outbound.Channels.FindCredentials(ctx, *conv.ChannelID)
		if credErr != nil {
			return nil, serviceerrors.MapRepository(credErr)
		}
		token, decryptErr := s.outbound.Cipher.Decrypt(creds.AccessTokenEnc)
		if decryptErr != nil {
			return nil, serviceerrors.NewValidation("channel", "invalid access token")
		}

		channelType := outboundChannelType(conv, ch)
		deliveryContent := content
		switch channelType {
		case conversation.ChannelInstagram:
			if s.outbound.Instagram == nil {
				return nil, serviceerrors.NewValidation("channel", "instagram outbound channel is not configured")
			}
			if s.outbound.Contacts == nil {
				return nil, serviceerrors.NewValidation("contact", "contact store is not configured")
			}
			deliveryContent, deliveryNotice = s.prepareInstagramReplyDelivery(ctx, teamID, conversationID, content, contentType, input)
			recipient, wamid, err = s.deliverInstagram(ctx, teamID, ch, conv.ContactID, token, deliveryContent, contentType, attachment, "")
		default:
			if s.outbound.WhatsApp == nil {
				return nil, serviceerrors.NewValidation("channel", "whatsapp outbound channel is not configured")
			}
			recipient, wamid, err = s.deliverWhatsApp(ctx, teamID, ch, conv.ContactID, token, content, contentType, attachment, replyToMID, input.ProviderMediaID)
		}
		if err != nil {
			return nil, err
		}
	}

	msg, err := s.createOutbound(ctx, teamID, userID, actorRole, conversationID, createOutboundInput{
		Content:          content,
		ContentType:      contentType,
		Recipient:        recipient,
		WAMID:            wamid,
		ReplyToWAMID:     nullableWAMID(replyToMID),
		ReplyToMessageID: s.resolveReplyTarget(ctx, teamID, conversationID, replyToMID),
		Attachment:       attachment,
		ClientMessageID:  input.ClientMessageID,
	})
	if err != nil {
		return nil, err
	}
	enriched := []Message{*msg}
	if err := s.enrichSenderNames(ctx, enriched); err != nil {
		return nil, err
	}
	*msg = enriched[0]
	if strings.TrimSpace(deliveryNotice) != "" {
		notice := deliveryNotice
		msg.DeliveryNotice = &notice
	}
	return msg, nil
}

func outboundChannelType(conv *conversation.Conversation, ch *channel.Channel) conversation.ChannelType {
	if conv != nil && conv.ChannelType == conversation.ChannelInstagram {
		return conversation.ChannelInstagram
	}
	if ch != nil && ch.ChannelType == channel.TypeInstagram {
		return conversation.ChannelInstagram
	}
	return conversation.ChannelWhatsApp
}

func (s *Service) deliverWhatsApp(
	ctx context.Context,
	teamID uuid.UUID,
	ch *channel.Channel,
	contactID uuid.UUID,
	token, content string,
	contentType ContentType,
	attachment *Attachment,
	replyToWAMID string,
	existingProviderMediaID string,
) (*string, *string, error) {
	if s.outbound == nil || s.outbound.WhatsAppAddresses == nil {
		return nil, nil, serviceerrors.NewValidation("contact", "whatsapp address resolver is not configured")
	}
	waRec, resolveErr := s.outbound.WhatsAppAddresses.Resolve(ctx, teamID, contactID)
	if resolveErr != nil {
		if errors.Is(resolveErr, camrecipient.ErrNoWhatsAppProvider) {
			return nil, nil, serviceerrors.NewValidation("contact", "whatsapp provider is required")
		}
		return nil, nil, serviceerrors.MapRepository(resolveErr)
	}
	phone := ""
	if waRec != nil {
		phone = strings.TrimSpace(waRec.PhoneNumber)
	}
	if phone == "" {
		return nil, nil, serviceerrors.NewValidation("contact", "whatsapp provider is required")
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return nil, nil, serviceerrors.NewValidation("channel", "whatsapp channel is not connected")
	}
	if s.outbound.WhatsApp == nil {
		return nil, nil, serviceerrors.NewValidation("channel", "whatsapp outbound channel is not configured")
	}

	creds := outbound.Credentials{
		AccessToken:   token,
		PhoneNumberID: strings.TrimSpace(*ch.PhoneNumberID),
	}
	replyToWAMID = strings.TrimSpace(replyToWAMID)

	var sendErr error
	var wamidValue string
	if attachment != nil {
		mediaID, mediaErr := s.resolveWhatsAppAttachmentMediaID(
			ctx, teamID, creds.PhoneNumberID, token, attachment, existingProviderMediaID,
		)
		if mediaErr != nil {
			return nil, nil, mediaErr
		}
		wamidValue, sendErr = s.outbound.WhatsApp.SendMedia(ctx, creds, outbound.MediaMessage{
			To:                phone,
			Type:              WhatsAppMessageType(contentType),
			ProviderMediaID:   mediaID,
			Caption:           content,
			Filename:          attachment.Filename,
			ReplyToProviderID: replyToWAMID,
		})
	} else {
		wamidValue, sendErr = s.outbound.WhatsApp.SendText(ctx, creds, outbound.TextMessage{
			To:                phone,
			Body:              content,
			ReplyToProviderID: replyToWAMID,
		})
	}
	if sendErr != nil {
		return nil, nil, serviceerrors.NewValidation("whatsapp", sendErr.Error())
	}

	recipient := phone
	return &recipient, &wamidValue, nil
}

// resolveWhatsAppAttachmentMediaID obtains a Cloud API media id via MediaManager.EnsureMedia.
// Compatibility: an already-valid provider media id skips EnsureMedia (no re-upload).
func (s *Service) resolveWhatsAppAttachmentMediaID(
	ctx context.Context,
	teamID uuid.UUID,
	phoneNumberID, accessToken string,
	attachment *Attachment,
	existingProviderMediaID string,
) (string, error) {
	if id := strings.TrimSpace(existingProviderMediaID); id != "" && isWhatsAppCloudMediaID(id) {
		return id, nil
	}
	if s.outbound == nil || s.outbound.Media == nil {
		return "", serviceerrors.NewValidation("media", "media manager is not configured for WhatsApp attachments")
	}
	if attachment == nil || attachment.StorageObjectID == uuid.Nil {
		return "", serviceerrors.NewValidation("attachment", "storage_object_id is required")
	}

	ref, err := s.outbound.Media.EnsureMedia(ctx, media.EnsureInput{
		TeamID:        teamID,
		AssetID:       attachment.StorageObjectID,
		PhoneNumberID: strings.TrimSpace(phoneNumberID),
		AccessToken:   strings.TrimSpace(accessToken),
	})
	if err != nil {
		return "", serviceerrors.NewValidation("whatsapp", err.Error())
	}
	if ref == nil || strings.TrimSpace(ref.ProviderMediaID) == "" {
		return "", serviceerrors.NewValidation("whatsapp", "media manager returned empty provider media id")
	}
	return strings.TrimSpace(ref.ProviderMediaID), nil
}

func isWhatsAppCloudMediaID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) deliverInstagram(
	ctx context.Context,
	teamID uuid.UUID,
	ch *channel.Channel,
	contactID uuid.UUID,
	token, content string,
	contentType ContentType,
	attachment *Attachment,
	replyToMID string,
) (*string, *string, error) {
	if s.outbound == nil || s.outbound.Contacts == nil {
		return nil, nil, serviceerrors.NewValidation("contact", "contact store is not configured")
	}
	// Prefer Canonical Provider / linked IGSID — never rely on Contact.Phone as primary.
	scopedID, resolveErr := s.outbound.Contacts.ResolveInstagramScopedID(ctx, teamID, contactID)
	scopedID = strings.TrimSpace(scopedID)
	ok := resolveErr == nil && contact.IsValidInstagramScopedID(scopedID)
	if !ok {
		return nil, nil, serviceerrors.NewValidation("contact", "instagram recipient id is required (linked Instagram profile IGSID)")
	}
	igUserID := ""
	if ch.IGUserID != nil {
		igUserID = strings.TrimSpace(*ch.IGUserID)
	}
	if igUserID == "" {
		return nil, nil, serviceerrors.NewValidation("channel", "instagram channel is not connected")
	}
	if s.outbound.Instagram == nil {
		return nil, nil, serviceerrors.NewValidation("channel", "instagram outbound channel is not configured")
	}

	creds := outbound.Credentials{
		AccessToken: token,
		IGUserID:    igUserID,
	}
	replyToMID = strings.TrimSpace(replyToMID)
	sendFn := func(replyTarget string) (string, error) {
		if attachment != nil {
			if s.outbound.MediaURLs == nil {
				return "", serviceerrors.NewValidation("attachment", "media proxy is not configured")
			}
			mediaURL, urlErr := s.outbound.MediaURLs.Issue(teamID, attachment.StorageObjectID)
			if urlErr != nil {
				return "", serviceerrors.NewValidation("attachment", "unable to create media url: "+urlErr.Error())
			}
			caption := strings.TrimSpace(content)
			if caption == "" || strings.EqualFold(caption, strings.TrimSpace(attachment.Filename)) {
				caption = ""
			}
			return s.outbound.Instagram.SendMedia(ctx, creds, outbound.MediaMessage{
				To:                scopedID,
				Type:              WhatsAppMessageType(contentType),
				MediaURL:          mediaURL,
				Caption:           caption,
				ReplyToProviderID: replyTarget,
				MIMEType:          attachment.MimeType,
			})
		}
		return s.outbound.Instagram.SendText(ctx, creds, outbound.TextMessage{
			To:                scopedID,
			Body:              content,
			ReplyToProviderID: replyTarget,
		})
	}
	mid, sendErr := sendFn(replyToMID)
	if sendErr != nil {
		return nil, nil, serviceerrors.NewValidation("instagram", sendErr.Error())
	}

	recipient := "ig:" + scopedID
	return &recipient, &mid, nil
}

type createOutboundInput struct {
	Content          string
	ContentType      ContentType
	Recipient        *string
	WAMID            *string
	ReplyToWAMID     *string
	ReplyToMessageID *uuid.UUID
	Attachment       *Attachment
	ClientMessageID  *uuid.UUID
}

func (s *Service) createOutbound(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, input createOutboundInput) (*Message, error) {
	conv, err := s.conversations.FindByID(ctx, teamID, conversationID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	now := time.Now().UTC()
	sentStatus := StatusSent
	m := &Message{
		ID:               uuid.New(),
		ClientMessageID:  input.ClientMessageID,
		ConversationID:   conversationID,
		TeamID:           teamID,
		ChannelID:        conv.ChannelID,
		Direction:        DirectionOutbound,
		Sender:           userID.String(),
		Recipient:        input.Recipient,
		Content:          input.Content,
		ContentType:      input.ContentType,
		WAMID:            input.WAMID,
		ReplyToWAMID:     input.ReplyToWAMID,
		ReplyToMessageID: input.ReplyToMessageID,
		Status:           &sentStatus,
		ReceivedAt:       now,
		CreatedAt:        now,
	}
	if m.ContentType == "" {
		m.ContentType = ContentText
	}
	if err := serviceerrors.MapRepository(s.messages.Create(ctx, m)); err != nil {
		return nil, err
	}
	if input.Attachment != nil && s.attachments != nil {
		if err := s.attachments.Create(ctx, &MessageAttachment{
			ID:              uuid.New(),
			MessageID:       m.ID,
			TeamID:          teamID,
			StorageObjectID: input.Attachment.StorageObjectID,
			MimeType:        input.Attachment.MimeType,
			SizeBytes:       input.Attachment.SizeBytes,
			Filename:        input.Attachment.Filename,
		}); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		m.Attachment = input.Attachment
	}
	return m, nil
}

func filenameFromObject(objectMeta *storageclient.StoredObject) string {
	if objectMeta == nil {
		return "attachment"
	}
	parts := strings.Split(objectMeta.ObjectKey, "/")
	if len(parts) == 0 {
		return "attachment"
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	if name == "" {
		return "attachment"
	}
	return name
}

func nullableWAMID(v string) *string {
	value := strings.TrimSpace(v)
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) revokeOnProvider(ctx context.Context, teamID uuid.UUID, m *Message) error {
	if m == nil || m.ChannelID == nil || m.WAMID == nil || strings.TrimSpace(*m.WAMID) == "" {
		return nil
	}
	if s.outbound == nil || s.outbound.WhatsApp == nil || s.outbound.Channels == nil || s.outbound.Cipher == nil {
		return nil
	}
	ch, err := s.outbound.Channels.FindByID(ctx, teamID, *m.ChannelID)
	if err != nil || ch == nil {
		return nil
	}
	if outboundChannelType(nil, ch) == conversation.ChannelInstagram {
		return nil
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return nil
	}
	credsRow, err := s.outbound.Channels.FindCredentials(ctx, *m.ChannelID)
	if err != nil {
		return nil
	}
	token, err := s.outbound.Cipher.Decrypt(credsRow.AccessTokenEnc)
	if err != nil {
		return nil
	}
	return s.outbound.WhatsApp.DeleteMessage(ctx, outbound.Credentials{
		AccessToken:   token,
		PhoneNumberID: strings.TrimSpace(*ch.PhoneNumberID),
	}, *m.WAMID)
}
