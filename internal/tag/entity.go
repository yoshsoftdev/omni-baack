package tag

import (
	"time"

	"github.com/google/uuid"
)

type Tag struct {
	ID           uuid.UUID
	TeamID       uuid.UUID
	Name         string
	Color        string
	Description  *string
	ContactCount int
	CreatedBy    *uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
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
