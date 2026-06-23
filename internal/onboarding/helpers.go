package onboarding

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func findOrCreateContact(ctx context.Context, contacts contact.Repository, teamID uuid.UUID, rawPhone, name string) (*contact.Contact, error) {
	phoneNorm := phone.Normalize(rawPhone)
	existing, err := contacts.FindByPhone(ctx, teamID, phoneNorm)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	c := &contact.Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		Name:            strings.TrimSpace(name),
		Phone:           phoneNorm,
		Source:          contact.SourceWhatsApp,
		MarketingStatus: contact.MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if c.Name == "" {
		c.Name = phoneNorm
	}
	if err := contacts.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func findOrCreateConversation(ctx context.Context, convos conversation.Repository, teamID, contactID, channelID uuid.UUID) (*conversation.Conversation, error) {
	channelPtr := &channelID
	existing, err := convos.FindByContactChannel(ctx, teamID, contactID, channelPtr)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	conv := &conversation.Conversation{
		ID:          uuid.New(),
		TeamID:      teamID,
		ContactID:   contactID,
		ChannelID:   channelPtr,
		ChannelType: conversation.ChannelWhatsApp,
		Status:      conversation.StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := convos.Create(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func touchConversationFromMessage(ctx context.Context, convos conversation.Repository, conv *conversation.Conversation, body string, inbound bool) error {
	preview := strings.TrimSpace(body)
	if preview == "" {
		preview = "New message"
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}
	now := time.Now().UTC()
	if err := convos.UpdateLastMessage(ctx, conv.TeamID, conv.ID, conversation.LastMessageUpdate{
		Preview: preview,
		At:      now,
	}); err != nil {
		return err
	}
	unread := conv.UnreadCount
	if inbound {
		unread++
	}
	return convos.UpdateUnreadCount(ctx, conv.TeamID, conv.ID, unread)
}
