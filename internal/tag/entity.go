package tag

import (
	"time"

	"github.com/google/uuid"
)

type Tag struct {
	ID           uuid.UUID  `json:"id"`
	TeamID       uuid.UUID  `json:"team_id"`
	Name         string     `json:"name"`
	Color        string     `json:"color"`
	Description  *string    `json:"description,omitempty"`
	ContactCount int        `json:"contact_count"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	Search string
}

type CreateInput struct {
	Name        string
	Color       string
	Description *string
}

type UpdateInput struct {
	Name        *string
	Color       *string
	Description *string
}
