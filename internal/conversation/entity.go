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
	ViewClosed     InboxView = "closed"
)

type Conversation struct {
	ID                    uuid.UUID   `json:"id"`
	TeamID                uuid.UUID   `json:"team_id"`
	ContactID             uuid.UUID   `json:"contact_id"`
	ProviderProfileID     *uuid.UUID  `json:"provider_profile_id,omitempty"`
	ExternalThreadID      string      `json:"external_thread_id"`
	Provider              string      `json:"provider"`
	LinkedProviders       []string    `json:"linked_providers"`
	IsSourceOfTruth       bool        `json:"is_source_of_truth"`
	LinkedAccountCount    int         `json:"linked_account_count"`
	ChannelID             *uuid.UUID  `json:"channel_id,omitempty"`
	ChannelType           ChannelType `json:"channel_type"`
	Status                Status      `json:"status"`
	AssignedTo            *uuid.UUID  `json:"assigned_to,omitempty"`
	UnreadCount           int         `json:"unread_count"`
	LastMessagePreview    string      `json:"last_message_preview"`
	LastMessageAt         *time.Time  `json:"last_message_at,omitempty"`
	FirstResponseAt       *time.Time  `json:"first_response_at,omitempty"`
	LastCustomerMessageAt *time.Time  `json:"last_customer_message_at,omitempty"`
	LastAgentMessageAt    *time.Time  `json:"last_agent_message_at,omitempty"`
	ResolutionAt          *time.Time  `json:"resolution_at,omitempty"`
	WaitingSince          *time.Time  `json:"waiting_since,omitempty"`
	BasePriority          int         `json:"base_priority"`
	PriorityScore         int64       `json:"priority_score"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
	DeletedAt             *time.Time  `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	Status      *Status
	AssignedTo  *uuid.UUID
	ChannelID   *uuid.UUID
	ChannelType *ChannelType
	Search      string
	View        InboxView
	ActorID     *uuid.UUID
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
	WaitingSince          *time.Time
	ClearWaitingSince     bool
	BasePriority          *int
	PriorityScore         *int64
}

// ListCursor is the keyset pagination position for inbox ordering.
type ListCursor struct {
	PriorityScore int64
	LastMessageAt *time.Time
	ID            uuid.UUID
}

// CursorPage is a cursor-paginated conversation result.
type CursorPage struct {
	Items      []Conversation
	NextCursor *ListCursor
	HasMore    bool
}
