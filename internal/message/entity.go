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
	WAMID          *string
	Direction      string
	Content        string
	SenderPhone    string
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
	SentAt         time.Time  `json:"sent_at"`
	WAMID          *string    `json:"wamid,omitempty"`
}

type Event struct {
	Type string   `json:"type"`
	Data Response `json:"data"`
}

func ToResponse(m *Message) Response {
	return Response{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Direction:      m.Direction,
		Content:        m.Content,
		SenderPhone:    m.SenderPhone,
		SentAt:         m.SentAt,
		WAMID:          m.WAMID,
	}
}
