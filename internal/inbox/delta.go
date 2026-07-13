package inbox

import (
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

// ConversationDelta is the slim WebSocket payload for list updates.
type ConversationDelta struct {
	ConversationID   uuid.UUID  `json:"conversation_id"`
	LastMessage      string     `json:"last_message,omitempty"`
	LastMessageAt    *time.Time `json:"last_message_at,omitempty"`
	PriorityScore    int64      `json:"priority_score"`
	WaitingSince     *time.Time `json:"waiting_since,omitempty"`
	UnreadCount      int        `json:"unread_count"`
	Status           string     `json:"status,omitempty"`
	AssignedTo       *uuid.UUID `json:"assigned_to,omitempty"`
	ContactID        uuid.UUID  `json:"contact_id,omitempty"`
	ContactName      string     `json:"contact_name,omitempty"`
	ContactPhone     string     `json:"contact_phone,omitempty"`
	ChannelType      string     `json:"channel_type,omitempty"`
}

// ToDelta builds a slim realtime payload from a conversation.
func ToDelta(item conversation.Item) ConversationDelta {
	conv := item.Conversation
	return ConversationDelta{
		ConversationID: conv.ID,
		LastMessage:    conv.LastMessagePreview,
		LastMessageAt:  conv.LastMessageAt,
		PriorityScore:  conv.PriorityScore,
		WaitingSince:   conv.WaitingSince,
		UnreadCount:    conv.UnreadCount,
		Status:         string(conv.Status),
		AssignedTo:     conv.AssignedTo,
		ContactID:      conv.ContactID,
		ContactName:    item.ContactName,
		ContactPhone:   item.ContactPhone,
		ChannelType:    string(conv.ChannelType),
	}
}

// ToCreatedPayload returns the full insertable row for conversation.created.
func ToCreatedPayload(item conversation.Item) ConversationDelta {
	delta := ToDelta(item)
	if item.DeviceID != "" {
		// expose channel id via channel_type field usage on frontend mapper
		_ = item.DeviceID
	}
	return delta
}
