package message

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// MetaMessenger sends WhatsApp text messages via Meta Cloud API.
type MetaMessenger interface {
	SendTextMessage(ctx context.Context, phoneNumberID, accessToken, to, body string) (string, error)
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
}

// SendReply persists an agent reply and delivers it through WhatsApp when configured.
func (s *Service) SendReply(ctx context.Context, teamID, userID uuid.UUID, actorRole string, conversationID uuid.UUID, content string) (*Message, error) {
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
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, serviceerrors.NewValidation("content", "is required")
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

		wamidValue, sendErr := s.outbound.Meta.SendTextMessage(ctx, *ch.PhoneNumberID, token, phone, content)
		if sendErr != nil {
			return nil, serviceerrors.NewValidation("whatsapp", sendErr.Error())
		}

		recipient = &phone
		wamid = &wamidValue
	}

	return s.Create(ctx, teamID, userID, actorRole, CreateInput{
		ConversationID: conversationID,
		Direction:      DirectionOutbound,
		Sender:         userID.String(),
		Recipient:      recipient,
		Content:        content,
		WAMID:          wamid,
	})
}
