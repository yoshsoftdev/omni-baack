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
	ID             uuid.UUID       `json:"id"`
	ChannelID      uuid.UUID       `json:"channel_id"`
	TeamID         uuid.UUID       `json:"team_id,omitempty"`
	WABAID         string          `json:"waba_id"`
	MetaTemplateID string          `json:"meta_template_id"`
	Name           string          `json:"name"`
	Language       string          `json:"language"`
	Category       *string         `json:"category,omitempty"`
	Status         Status          `json:"status"`
	Preview        *string         `json:"preview,omitempty"`
	Components     json.RawMessage `json:"components,omitempty"`
	VariableCount  int             `json:"variable_count,omitempty"`
	LastSyncAt     *time.Time      `json:"last_sync_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	ChannelID *uuid.UUID
	Status    *Status
	Search    string
}
