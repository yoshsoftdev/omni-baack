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
	SourceWhatsApp Source = "whatsapp"
	SourceImport   Source = "import"
	SourceManual   Source = "manual"
	SourceAPI      Source = "api"
)

type Contact struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	Name            string
	Phone           string
	Email           *string
	Notes           *string
	City            *string
	Language        *string
	CustomerType    *string
	Source          Source
	MarketingStatus MarketingStatus
	LastActivityAt  *time.Time
	ImportJobID     *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type ListFilter struct {
	Search          string
	TagID           *uuid.UUID
	SegmentID       *uuid.UUID
	MarketingStatus *MarketingStatus
}
