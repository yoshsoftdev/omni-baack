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

type DeletedBySource string

const (
	DeletedBySourceWAUser DeletedBySource = "user_wa"
	DeletedBySourceAgent  DeletedBySource = "agent_app"
	DeletedBySourceSystem DeletedBySource = "system"
)

type Message struct {
	ID               uuid.UUID   `json:"id"`
	ClientMessageID  *uuid.UUID  `json:"client_message_id,omitempty"`
	ConversationID   uuid.UUID   `json:"conversation_id"`
	TeamID         uuid.UUID   `json:"team_id"`
	ChannelID      *uuid.UUID  `json:"channel_id,omitempty"`
	Direction      Direction   `json:"direction"`
	Sender         string      `json:"sender"`
	SenderName     *string     `json:"sender_name,omitempty"`
	Recipient      *string     `json:"recipient,omitempty"`
	Content        string      `json:"content"`
	ContentType    ContentType `json:"content_type"`
	WAMID          *string     `json:"wamid,omitempty"`
	ReplyToWAMID   *string     `json:"reply_to_wamid,omitempty"`
	ReplyToMessageID *uuid.UUID `json:"reply_to_message_id,omitempty"`
	Status         *Status     `json:"status,omitempty"`
	IsDeleted      bool        `json:"is_deleted"`
	DeletedAt      *time.Time  `json:"deleted_at,omitempty"`
	DeletedBySource *DeletedBySource `json:"deleted_by_source,omitempty"`
	RawPayload     []byte      `json:"-"`
	Attachment     *Attachment `json:"attachment,omitempty"`
	ReceivedAt     time.Time   `json:"received_at"`
	CreatedAt      time.Time   `json:"created_at"`
	DeliveryNotice *string     `json:"delivery_notice,omitempty"`
}

type ListFilter struct {
	Direction *Direction
	Since     *time.Time
}
