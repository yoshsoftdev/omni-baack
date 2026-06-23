package template

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusApproved Status = "APPROVED"
	StatusPending  Status = "PENDING"
	StatusRejected Status = "REJECTED"
)

// Template is a Meta WhatsApp template synced per channel.
type Template struct {
	ID             uuid.UUID
	ChannelID      uuid.UUID
	TeamID         uuid.UUID
	WABAID         string
	MetaTemplateID string
	Name           string
	Language       string
	Category       *string
	Status         Status
	Preview        *string
	Components     json.RawMessage
	VariableCount  int
	LastSyncAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type ListFilter struct {
	ChannelID *uuid.UUID
	Status    *Status
	Search    string
}
