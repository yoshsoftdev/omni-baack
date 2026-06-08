package template

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Template struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	DeviceID        uuid.UUID
	WabaID          string
	MetaTemplateID  string
	Name            string
	Language        string
	Category        *string
	Status          *string
	Components      json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Response struct {
	ID             uuid.UUID       `json:"id"`
	DeviceID       uuid.UUID       `json:"device_id"`
	WabaID         string          `json:"waba_id"`
	MetaTemplateID string          `json:"meta_template_id"`
	Name           string          `json:"name"`
	Language       string          `json:"language"`
	Category       *string         `json:"category,omitempty"`
	Status         *string         `json:"status,omitempty"`
	Preview        string          `json:"preview,omitempty"`
	Components     json.RawMessage `json:"components"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func ToResponse(t *Template) Response {
	return Response{
		ID:             t.ID,
		DeviceID:       t.DeviceID,
		WabaID:         t.WabaID,
		MetaTemplateID: t.MetaTemplateID,
		Name:           t.Name,
		Language:       t.Language,
		Category:       t.Category,
		Status:         t.Status,
		Preview:        ExtractPreview(t.Components),
		Components:     t.Components,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}
