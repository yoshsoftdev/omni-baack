package contact

import (
	"time"

	"github.com/google/uuid"
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
)

// TagRef is a lightweight tag assignment on a contact.
type TagRef struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

type Contact struct {
	ID              uuid.UUID       `json:"id"`
	TeamID          uuid.UUID       `json:"team_id"`
	Name            string          `json:"name"`
	Phone           string          `json:"phone"`
	Email           *string         `json:"email,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
	City            *string         `json:"city,omitempty"`
	Language        *string         `json:"language,omitempty"`
	CustomerType    *string         `json:"customer_type,omitempty"`
	Source          Source          `json:"source"`
	MarketingStatus MarketingStatus `json:"marketing_status"`
	Tags            []TagRef        `json:"tags"`
	LastActivityAt  *time.Time      `json:"last_activity_at,omitempty"`
	ImportJobID     *uuid.UUID      `json:"import_job_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	DeletedAt       *time.Time      `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	Search          string
	TagID           *uuid.UUID
	SegmentID       *uuid.UUID
	MarketingStatus *MarketingStatus
}
