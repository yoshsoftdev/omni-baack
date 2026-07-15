package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// FindOrCreateForInbound finds or creates a conversation for inbound Meta messages.
func (s *Service) FindOrCreateForInbound(ctx context.Context, teamID, contactID, channelID uuid.UUID, channelType ChannelType) (*Conversation, bool, error) {
	if channelType == "" {
		channelType = ChannelWhatsApp
	}
	if _, err := s.contacts.FindByID(ctx, teamID, contactID); err != nil {
		return nil, false, serviceerrors.MapRepository(err)
	}

	channelPtr := &channelID
	existing, err := s.conversations.FindByContactChannel(ctx, teamID, contactID, channelPtr)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	now := time.Now().UTC()
	c := &Conversation{
		ID:          uuid.New(),
		TeamID:      teamID,
		ContactID:   contactID,
		ChannelID:   channelPtr,
		ChannelType: channelType,
		Status:      StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.conversations.Create(ctx, c); err != nil {
		if existing, findErr := s.conversations.FindByContactChannel(ctx, teamID, contactID, channelPtr); findErr == nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	return c, true, nil
}

// TouchInbound updates conversation preview and unread count after inbound message.
func (s *Service) TouchInbound(ctx context.Context, teamID, conversationID uuid.UUID, preview string, unread int) error {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		preview = "New message"
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}
	now := time.Now().UTC()
	if err := s.conversations.UpdateLastMessage(ctx, teamID, conversationID, LastMessageUpdate{
		Preview: preview,
		At:      now,
	}); err != nil {
		return err
	}
	return s.conversations.UpdateUnreadCount(ctx, teamID, conversationID, unread)
}
