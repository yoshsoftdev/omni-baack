package message

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// MetaMessenger sends WhatsApp and Instagram messages via Meta APIs.
type MetaMessenger interface {
	SendTextMessage(ctx context.Context, phoneNumberID, accessToken, to, body string) (string, error)
	SendTextMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, body, replyToWAMID string) (string, error)
	UploadMedia(ctx context.Context, phoneNumberID, accessToken, mimeType, filename string, body io.Reader, size int64) (string, error)
	SendMediaMessage(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename string) (string, error)
	SendMediaMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename, replyToWAMID string) (string, error)
	DeleteMessage(ctx context.Context, phoneNumberID, accessToken, wamid string) error
	SendInstagramTextMessage(ctx context.Context, igUserID, accessToken, recipientScopedID, body, replyToMID string) (string, error)
	SendInstagramMediaMessage(ctx context.Context, igUserID, accessToken, recipientScopedID, contentType, mediaURL, caption, replyToMID string) (string, error)
}

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

// ContactStore loads contact phone numbers for outbound delivery.
type ContactStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*contact.Contact, error)
	ResolveInstagramScopedID(ctx context.Context, teamID, contactID uuid.UUID) (string, error)
}

// TokenDecryptor decrypts stored Meta access tokens.
type TokenDecryptor interface {
	Decrypt(ciphertext []byte) (string, error)
}

// OutboundDeps wires channel delivery for agent replies.
type OutboundDeps struct {
	Channels  ChannelStore
	Contacts  ContactStore
	Meta      MetaMessenger
	Cipher    TokenDecryptor
	Storage   StorageDownloader
	MediaURLs MediaURLIssuer
}

type SendInput struct {
	Content          string
	StorageObjectID  *uuid.UUID
	ReplyToWAMID     string
	ReplyToMessageID *uuid.UUID
	ClientMessageID  *uuid.UUID
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
		meta, metaErr := s.outbound.Storage.GetMetadata(ctx, teamID, *input.StorageObjectID)
		if metaErr != nil {
			return nil, serviceerrors.NewValidation("storage_object_id", "invalid storage object")
		}
		validatedType, validateErr := ValidateAttachmentForChannel(meta.MimeType, meta.SizeBytes, channelHint)
		if validateErr != nil {
			return nil, serviceerrors.NewValidation("attachment", validateErr.Error())
		}
		contentType = validatedType
		attachment = &Attachment{
			StorageObjectID: meta.ID,
			MimeType:        meta.MimeType,
			SizeBytes:       meta.SizeBytes,
			Filename:        filenameFromObject(meta),
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
		s.outbound.Meta != nil &&
		s.outbound.Contacts != nil &&
		s.outbound.Channels != nil &&
		s.outbound.Cipher != nil &&
		conv.ChannelID != nil {
		contactEntity, contactErr := s.outbound.Contacts.FindByID(ctx, teamID, conv.ContactID)
		if contactErr != nil {
			return nil, serviceerrors.MapRepository(contactErr)
		}

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
			deliveryContent, deliveryNotice = s.prepareInstagramReplyDelivery(ctx, teamID, conversationID, content, contentType, input)
			recipient, wamid, err = s.deliverInstagram(ctx, teamID, ch, contactEntity, token, deliveryContent, contentType, attachment, "")
		default:
			recipient, wamid, err = s.deliverWhatsApp(ctx, teamID, ch, contactEntity, token, content, contentType, attachment, replyToMID)
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
	contactEntity *contact.Contact,
	token, content string,
	contentType ContentType,
	attachment *Attachment,
	replyToWAMID string,
) (*string, *string, error) {
	phone := strings.TrimSpace(contactEntity.Phone)
	if phone == "" {
		return nil, nil, serviceerrors.NewValidation("contact", "phone is required")
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return nil, nil, serviceerrors.NewValidation("channel", "whatsapp channel is not connected")
	}

	var sendErr error
	var wamidValue string
	replyToWAMID = strings.TrimSpace(replyToWAMID)
	if attachment != nil {
		reader, _, downloadErr := s.outbound.Storage.Download(ctx, teamID, attachment.StorageObjectID)
		if downloadErr != nil {
			return nil, nil, serviceerrors.NewValidation("attachment", "unable to download storage object")
		}
		defer reader.Close()
		mediaID, uploadErr := s.outbound.Meta.UploadMedia(ctx, *ch.PhoneNumberID, token, attachment.MimeType, attachment.Filename, reader, attachment.SizeBytes)
		if uploadErr != nil {
			return nil, nil, serviceerrors.NewValidation("whatsapp", uploadErr.Error())
		}
		wamidValue, sendErr = s.outbound.Meta.SendMediaMessageWithContext(
			ctx,
			*ch.PhoneNumberID,
			token,
			phone,
			WhatsAppMessageType(contentType),
			mediaID,
			content,
			attachment.Filename,
			replyToWAMID,
		)
	} else {
		wamidValue, sendErr = s.outbound.Meta.SendTextMessageWithContext(
			ctx,
			*ch.PhoneNumberID,
			token,
			phone,
			content,
			replyToWAMID,
		)
	}
	if sendErr != nil {
		return nil, nil, serviceerrors.NewValidation("whatsapp", sendErr.Error())
	}

	recipient := phone
	return &recipient, &wamidValue, nil
}

func (s *Service) deliverInstagram(
	ctx context.Context,
	teamID uuid.UUID,
	ch *channel.Channel,
	contactEntity *contact.Contact,
	token, content string,
	contentType ContentType,
	attachment *Attachment,
	replyToMID string,
) (*string, *string, error) {
	scopedID, ok := contact.ParseInstagramScopedID(contactEntity.Phone)
	if !ok && s.outbound != nil && s.outbound.Contacts != nil {
		resolvedID, resolveErr := s.outbound.Contacts.ResolveInstagramScopedID(ctx, contactEntity.TeamID, contactEntity.ID)
		if resolveErr == nil {
			scopedID = strings.TrimSpace(resolvedID)
			ok = contact.IsValidInstagramScopedID(scopedID)
		}
	}
	if !ok || !contact.IsValidInstagramScopedID(scopedID) {
		return nil, nil, serviceerrors.NewValidation("contact", "instagram recipient id is required (linked Instagram profile IGSID)")
	}
	igUserID := ""
	if ch.IGUserID != nil {
		igUserID = strings.TrimSpace(*ch.IGUserID)
	}
	if igUserID == "" {
		return nil, nil, serviceerrors.NewValidation("channel", "instagram channel is not connected")
	}
	replyToMID = strings.TrimSpace(replyToMID)
	sendFn := func(replyTarget string) (string, error) {
		if attachment != nil {
			if s.outbound == nil || s.outbound.MediaURLs == nil {
				return "", serviceerrors.NewValidation("attachment", "media proxy is not configured")
			}
			if mimeErr := meta.ValidateInstagramOutboundMIME(attachment.MimeType); mimeErr != nil {
				return "", serviceerrors.NewValidation("attachment", mimeErr.Error())
			}
			mediaURL, urlErr := s.outbound.MediaURLs.Issue(teamID, attachment.StorageObjectID)
			if urlErr != nil {
				return "", serviceerrors.NewValidation("attachment", "unable to create media url: "+urlErr.Error())
			}
			caption := strings.TrimSpace(content)
			if caption == "" || strings.EqualFold(caption, strings.TrimSpace(attachment.Filename)) {
				caption = ""
			}
			return s.outbound.Meta.SendInstagramMediaMessage(
				ctx,
				igUserID,
				token,
				scopedID,
				WhatsAppMessageType(contentType),
				mediaURL,
				caption,
				replyTarget,
			)
		}
		return s.outbound.Meta.SendInstagramTextMessage(ctx, igUserID, token, scopedID, content, replyTarget)
	}
	mid, sendErr := sendFn(replyToMID)
	if sendErr != nil {
		return nil, nil, serviceerrors.NewValidation("instagram", sendErr.Error())
	}

	recipient := contactEntity.Phone
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

func filenameFromObject(meta *storageclient.StoredObject) string {
	if meta == nil {
		return "attachment"
	}
	parts := strings.Split(meta.ObjectKey, "/")
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
	if s.outbound == nil || s.outbound.Meta == nil || s.outbound.Channels == nil || s.outbound.Cipher == nil {
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
	creds, err := s.outbound.Channels.FindCredentials(ctx, *m.ChannelID)
	if err != nil {
		return nil
	}
	token, err := s.outbound.Cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return nil
	}
	return s.outbound.Meta.DeleteMessage(ctx, *ch.PhoneNumberID, token, *m.WAMID)
}
