package conversation

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusPending  Status = "pending"
	StatusResolved Status = "resolved"
)

type Conversation struct {
	ID                 uuid.UUID
	TeamID             uuid.UUID
	ContactID          uuid.UUID
	DeviceID           uuid.UUID
	Status             Status
	AssignedTo         *uuid.UUID
	LastMessageAt      *time.Time
	LastMessagePreview *string
	UnreadCount        int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type ListItem struct {
	ID                 uuid.UUID  `json:"id"`
	ContactID          uuid.UUID  `json:"contact_id"`
	ContactName        string     `json:"contact_name"`
	ContactPhone       string     `json:"contact_phone"`
	DeviceID           uuid.UUID  `json:"device_id"`
	Status             Status     `json:"status"`
	AssignedTo         *uuid.UUID `json:"assigned_to,omitempty"`
	AssigneeName       *string    `json:"assignee_name,omitempty"`
	AssigneeEmail      *string    `json:"assignee_email,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview *string    `json:"last_message_preview,omitempty"`
	UnreadCount        int        `json:"unread_count"`
}

type Detail struct {
	ListItem
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}
