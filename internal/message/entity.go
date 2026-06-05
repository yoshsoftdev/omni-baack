package message

import (
	"time"

	"github.com/google/uuid"
)

const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

type Message struct {
	ID             uuid.UUID
	TeamID         uuid.UUID
	ConversationID uuid.UUID
	DeviceID       *uuid.UUID
	ContactID      *uuid.UUID
	SentBy         *uuid.UUID
	WAMID          *string
	Direction      string
	Content        string
	SenderPhone    string
	SenderName     string
	SentAt         time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type Response struct {
	ID             uuid.UUID  `json:"id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	Direction      string     `json:"direction"`
	Content        string     `json:"content"`
	SenderPhone    string     `json:"sender_phone"`
	SenderName     *string    `json:"sender_name,omitempty"`
	SentBy         *uuid.UUID `json:"sent_by,omitempty"`
	SentAt         time.Time  `json:"sent_at"`
	WAMID          *string    `json:"wamid,omitempty"`
}

type Event struct {
	Type string   `json:"type"`
	Data Response `json:"data"`
}

func ToResponse(m *Message) Response {
	resp := Response{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Direction:      m.Direction,
		Content:        m.Content,
		SenderPhone:    m.SenderPhone,
		SentBy:         m.SentBy,
		SentAt:         m.SentAt,
		WAMID:          m.WAMID,
	}
	if m.SenderName != "" {
		resp.SenderName = &m.SenderName
	}
	return resp
}
