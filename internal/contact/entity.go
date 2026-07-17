package contact

import (
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
)

type MarketingStatus string

const (
	MarketingSubscribed   MarketingStatus = "subscribed"
	MarketingUnsubscribed MarketingStatus = "unsubscribed"
	MarketingUnknown      MarketingStatus = "unknown"
)

type Source string

const (
	SourceWhatsApp    Source = "whatsapp"
	SourceImport      Source = "import"
	SourceManual      Source = "manual"
	SourceAPI         Source = "api"
	SourceMetaInbound Source = "meta_inbound"
	SourceMetaSync    Source = "meta_sync"
	SourceInstagram   Source = "ig"
)

// TagRef is a lightweight tag assignment on a contact.
type TagRef struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// LinkedContact is the opposite side of a bidirectional contact relationship.
// Both contacts remain independent CRM records with their own provider profiles.
type LinkedContact struct {
	LinkID        uuid.UUID `json:"link_id"`
	ID            uuid.UUID `json:"id"`
	ContactName   string    `json:"contact_name"`
	CustomerName  string    `json:"customer_name"`
	Phone         string    `json:"phone"`
	Email         *string   `json:"email,omitempty"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	ProviderKinds []string  `json:"provider_kinds"`
	LinkedAt      time.Time `json:"linked_at"`
}

type Contact struct {
	ID                 uuid.UUID          `json:"id"`
	TeamID             uuid.UUID          `json:"team_id"`
	ContactName        string             `json:"contact_name"`
	CustomerName       string             `json:"customer_name"`
	Name               string             `json:"name,omitempty"` // deprecated alias of contact_name
	Phone              string             `json:"phone"`
	Email              *string            `json:"email,omitempty"`
	AvatarURL          *string            `json:"avatar_url,omitempty"`
	Notes              *string            `json:"notes,omitempty"`
	City               *string            `json:"city,omitempty"`
	Country            *string            `json:"country,omitempty"`
	Province           *string            `json:"province,omitempty"`
	Company            *string            `json:"company,omitempty"`
	JobTitle           *string            `json:"job_title,omitempty"`
	Language           *string            `json:"language,omitempty"`
	CustomerType       *string            `json:"customer_type,omitempty"`
	Source             Source             `json:"source"`
	MarketingStatus    MarketingStatus    `json:"marketing_status"`
	Tags               []TagRef           `json:"tags"`
	Providers          []provider.Profile `json:"providers"`
	LinkedContacts     []LinkedContact    `json:"linked_contacts"`
	CustomFields       []CustomFieldValue `json:"custom_fields,omitempty"`
	LastActivityAt     *time.Time         `json:"last_activity_at,omitempty"`
	ConversationsCount int                `json:"conversations_count"`
	LastCampaign       *LastCampaignRef   `json:"last_campaign,omitempty"`
	ImportJobID        *uuid.UUID         `json:"import_job_id,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          *time.Time         `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	Search          string
	TagID           *uuid.UUID
	SegmentID       *uuid.UUID
	MarketingStatus *MarketingStatus
}
