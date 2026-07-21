package quickreply

import (
	"time"

	"github.com/google/uuid"
)

type QuickReply struct {
	ID        uuid.UUID  `json:"id"`
	TeamID    uuid.UUID  `json:"team_id"`
	Category  string     `json:"category"`
	Content   string     `json:"content"`
	Text      string     `json:"text"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type ListFilter struct {
	Search string
}

type CreateInput struct {
	Category string
	Content  string
}

type UpdateInput struct {
	Category *string
	Content  *string
}
