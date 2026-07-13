package message

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// MetaMessenger sends WhatsApp messages via Meta Cloud API.
type MetaMessenger interface {
	SendTextMessage(ctx context.Context, phoneNumberID, accessToken, to, body string) (string, error)
	SendTextMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, body, replyToWAMID string) (string, error)
	UploadMedia(ctx context.Context, phoneNumberID, accessToken, mimeType, filename string, body io.Reader, size int64) (string, error)
	SendMediaMessage(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename string) (string, error)
	SendMediaMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename, replyToWAMID string) (string, error)
	DeleteMessage(ctx context.Context, phoneNumberID, accessToken, wamid string) error
}

// StorageDownloader fetches uploaded objects from storage-service.
type StorageDownloader interface {
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *storageclient.StoredObject, error)
	GetMetadata(ctx context.Context, teamID, objectID uuid.UUID) (*storageclient.StoredObject, error)
}

// ChannelStore loads channel credentials for outbound delivery.
type ChannelStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*channel.Channel, error)
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*channel.Credentials, error)
}

// ContactStore loads contact phone numbers for outbound delivery.
type ContactStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*contact.Contact, error)
}

// TokenDecryptor decrypts stored Meta access tokens.
type TokenDecryptor interface {
	Decrypt(ciphertext []byte) (string, error)
}

// OutboundDeps wires WhatsApp delivery for agent replies.
type OutboundDeps struct {
	Channels ChannelStore
	Contacts ContactStore
	Meta     MetaMessenger
	Cipher   TokenDecryptor
	Storage  StorageDownloader
}

type SendInput struct {
	Content         string
	StorageObjectID *uuid.UUID
	ReplyToWAMID    string
	ClientMessageID *uuid.UUID
}

// SendReply persists an agent reply and delivers it through WhatsApp when configured.
func (s *Service) SendReply(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, content string) (*Message, error) {
	return s.SendMessage(ctx, teamID, userID, actorRole, conversationID, SendInput{Content: content})
}

// SendMessage sends a text or media message through WhatsApp.
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
		validatedType, validateErr := ValidateAttachment(meta.MimeType, meta.SizeBytes)
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
		phone := strings.TrimSpace(contactEntity.Phone)
		if phone == "" {
			return nil, serviceerrors.NewValidation("contact", "phone is required")
		}

		ch, channelErr := s.outbound.Channels.FindByID(ctx, teamID, *conv.ChannelID)
		if channelErr != nil {
			return nil, serviceerrors.MapRepository(channelErr)
		}
		if ch.Status != channel.StatusConnected || ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
			return nil, serviceerrors.NewValidation("channel", "whatsapp channel is not connected")
		}

		creds, credErr := s.outbound.Channels.FindCredentials(ctx, *conv.ChannelID)
		if credErr != nil {
			return nil, serviceerrors.MapRepository(credErr)
		}
		token, decryptErr := s.outbound.Cipher.Decrypt(creds.AccessTokenEnc)
		if decryptErr != nil {
			return nil, serviceerrors.NewValidation("channel", "invalid access token")
		}

		var sendErr error
		var wamidValue string
		replyToWAMID := strings.TrimSpace(input.ReplyToWAMID)
		if attachment != nil {
			reader, _, downloadErr := s.outbound.Storage.Download(ctx, teamID, attachment.StorageObjectID)
			if downloadErr != nil {
				return nil, serviceerrors.NewValidation("attachment", "unable to download storage object")
			}
			defer reader.Close()
			mediaID, uploadErr := s.outbound.Meta.UploadMedia(ctx, *ch.PhoneNumberID, token, attachment.MimeType, attachment.Filename, reader, attachment.SizeBytes)
			if uploadErr != nil {
				return nil, serviceerrors.NewValidation("whatsapp", uploadErr.Error())
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
			return nil, serviceerrors.NewValidation("whatsapp", sendErr.Error())
		}

		recipient = &phone
		wamid = &wamidValue
	}

	msg, err := s.createOutbound(ctx, teamID, userID, actorRole, conversationID, createOutboundInput{
		Content:          content,
		ContentType:      contentType,
		Recipient:        recipient,
		WAMID:            wamid,
		ReplyToWAMID:     nullableWAMID(input.ReplyToWAMID),
		ReplyToMessageID: s.resolveReplyTarget(ctx, teamID, conversationID, input.ReplyToWAMID),
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
	return msg, nil
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
	if err != nil || ch == nil || ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
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
