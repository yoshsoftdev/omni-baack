package conversation

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusAssigned Status = "assigned"
	StatusClosed   Status = "closed"
)

type ChannelType string

const (
	ChannelWhatsApp  ChannelType = "whatsapp"
	ChannelInstagram ChannelType = "instagram"
	ChannelOther     ChannelType = "other"
)

type Conversation struct {
	ID                 uuid.UUID
	TeamID             uuid.UUID
	ContactID          uuid.UUID
	ChannelID          *uuid.UUID
	ChannelType        ChannelType
	Status             Status
	AssignedTo         *uuid.UUID
	UnreadCount        int
	LastMessagePreview string
	LastMessageAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type ListFilter struct {
	Status     *Status
	AssignedTo *uuid.UUID
	ChannelID  *uuid.UUID
	Search     string
}

type LastMessageUpdate struct {
	Preview string
	At      time.Time
}
