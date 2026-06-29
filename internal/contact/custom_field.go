package contact

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type FieldType string

const (
	FieldTypeText    FieldType = "text"
	FieldTypeNumber  FieldType = "number"
	FieldTypeDate    FieldType = "date"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeSelect  FieldType = "select"
)

type FieldDefinition struct {
	ID        uuid.UUID       `json:"id"`
	TeamID    uuid.UUID       `json:"team_id"`
	FieldKey  string          `json:"field_key"`
	Label     string          `json:"label"`
	FieldType FieldType       `json:"field_type"`
	Options   json.RawMessage `json:"options,omitempty"`
	SortOrder int             `json:"sort_order"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type CustomFieldValue struct {
	ID        uuid.UUID       `json:"id"`
	FieldKey  string          `json:"field_key"`
	Label     string          `json:"label"`
	FieldType FieldType       `json:"field_type"`
	Options   json.RawMessage `json:"options,omitempty"`
	Value     json.RawMessage `json:"value"`
}

type LastCampaignRef struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	SentAt time.Time `json:"sent_at"`
}

type ContactStats struct {
	ConversationsCount int              `json:"conversations_count"`
	LastCampaign       *LastCampaignRef `json:"last_campaign,omitempty"`
}

type CreateFieldDefinitionInput struct {
	FieldKey  string
	Label     string
	FieldType FieldType
	Options   json.RawMessage
	SortOrder int
}

type UpdateFieldDefinitionInput struct {
	Label     *string
	FieldType *FieldType
	Options   json.RawMessage
	SortOrder *int
}
