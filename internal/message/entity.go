package message

import (
	"time"

	"github.com/google/uuid"
)

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type ContentType string

const (
	ContentText     ContentType = "text"
	ContentImage    ContentType = "image"
	ContentVideo    ContentType = "video"
	ContentAudio    ContentType = "audio"
	ContentDocument ContentType = "document"
	ContentTemplate ContentType = "template"
	ContentOther    ContentType = "other"
)

type Status string

const (
	StatusSent      Status = "sent"
	StatusDelivered Status = "delivered"
	StatusRead      Status = "read"
	StatusFailed    Status = "failed"
)

type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	TeamID         uuid.UUID
	ChannelID      *uuid.UUID
	Direction      Direction
	Sender         string
	Recipient      *string
	Content        string
	ContentType    ContentType
	WAMID          *string
	Status         *Status
	RawPayload     []byte
	ReceivedAt     time.Time
	CreatedAt      time.Time
}

type ListFilter struct {
	Direction *Direction
	Since     *time.Time
}
