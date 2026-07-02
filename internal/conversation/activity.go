package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ActivityType string

const (
	ActivityAssigned   ActivityType = "assigned"
	ActivityReassigned ActivityType = "reassigned"
	ActivityOpened     ActivityType = "opened"
	ActivityClosed     ActivityType = "closed"
	ActivityReopened   ActivityType = "reopened"
	ActivityNoteAdded  ActivityType = "note_added"
)

type Activity struct {
	ID             uuid.UUID       `json:"id"`
	TeamID         uuid.UUID       `json:"team_id"`
	ConversationID uuid.UUID       `json:"conversation_id"`
	Type           ActivityType    `json:"type"`
	ActorID        *uuid.UUID      `json:"actor_id,omitempty"`
	ActorName      string          `json:"actor_name,omitempty"`
	Description    string          `json:"description"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Note struct {
	ID             uuid.UUID  `json:"id"`
	TeamID         uuid.UUID  `json:"team_id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	AuthorID       uuid.UUID  `json:"author_id"`
	AuthorName     string     `json:"author_name"`
	Content        string     `json:"content"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}
