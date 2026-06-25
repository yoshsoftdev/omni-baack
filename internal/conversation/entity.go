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
	StatusClosed   Status = "closed"
	// StatusAssigned is legacy; migrated to open in 000019.
	StatusAssigned Status = "assigned"
)

type ChannelType string

const (
	ChannelWhatsApp  ChannelType = "whatsapp"
	ChannelInstagram ChannelType = "instagram"
	ChannelOther     ChannelType = "other"
)

type InboxView string

const (
	ViewAll        InboxView = "all"
	ViewMine       InboxView = "mine"
	ViewUnassigned InboxView = "unassigned"
	ViewUnread     InboxView = "unread"
)

type Conversation struct {
	ID                    uuid.UUID
	TeamID                uuid.UUID
	ContactID             uuid.UUID
	ChannelID             *uuid.UUID
	ChannelType           ChannelType
	Status                Status
	AssignedTo            *uuid.UUID
	UnreadCount           int
	LastMessagePreview    string
	LastMessageAt         *time.Time
	FirstResponseAt       *time.Time
	LastCustomerMessageAt *time.Time
	LastAgentMessageAt    *time.Time
	ResolutionAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

type ListFilter struct {
	Status     *Status
	AssignedTo *uuid.UUID
	ChannelID  *uuid.UUID
	Search     string
	View       InboxView
	ActorID    *uuid.UUID
}

type LastMessageUpdate struct {
	Preview string
	At      time.Time
}

// InboxSyncUpdate applies inbox synchronization fields atomically.
type InboxSyncUpdate struct {
	Status                Status
	UnreadCount           int
	LastMessagePreview    string
	LastMessageAt         time.Time
	LastCustomerMessageAt *time.Time
	LastAgentMessageAt    *time.Time
	SetFirstResponseAt    bool
	ClearResolutionAt     bool
	SetResolutionAt       bool
	TouchLastMessage      bool
}
