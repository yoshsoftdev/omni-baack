package provider

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Profile is one provider-scoped external customer identity.
type Profile struct {
	ID              uuid.UUID       `json:"id"`
	TeamID          uuid.UUID       `json:"team_id"`
	ContactID       uuid.UUID       `json:"contact_id"`
	Provider        Kind            `json:"provider"`
	ProviderUserID  string          `json:"provider_user_id"`
	ChannelID       *uuid.UUID      `json:"channel_id,omitempty"`
	DisplayName     string          `json:"display_name,omitempty"`
	Username        string          `json:"username,omitempty"`
	Phone           string          `json:"phone,omitempty"`
	AvatarURL       string          `json:"avatar_url,omitempty"`
	Email           string          `json:"email,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	ConnectedAt     time.Time       `json:"connected_at"`
	LastSyncedAt    *time.Time      `json:"last_synced_at,omitempty"`
	LastObservedAt  *time.Time      `json:"last_observed_at,omitempty"`
	Status          Status          `json:"status"`
	IsSourceOfTruth bool            `json:"is_source_of_truth"`
	ConversationIDs []uuid.UUID     `json:"conversation_ids,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ActivityType string

const (
	ActivityLinked               ActivityType = "provider_linked"
	ActivityUnlinked             ActivityType = "provider_unlinked"
	ActivitySourceOfTruthChanged ActivityType = "source_of_truth_changed"
)

type Activity struct {
	ID                uuid.UUID       `json:"id"`
	TeamID            uuid.UUID       `json:"team_id"`
	ContactID         uuid.UUID       `json:"contact_id"`
	ProviderProfileID *uuid.UUID      `json:"provider_profile_id,omitempty"`
	Type              ActivityType    `json:"type"`
	ActorID           *uuid.UUID      `json:"actor_id,omitempty"`
	ActorName         string          `json:"actor_name,omitempty"`
	Description       string          `json:"description"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
}
