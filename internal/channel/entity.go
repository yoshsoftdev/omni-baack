package channel

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
)

type ConnectionType string

const (
	ConnectionManual       ConnectionType = "manual"
	ConnectionEmbeddedCoex ConnectionType = "embedded_coex"
	ConnectionOAuth        ConnectionType = "oauth"
)

type ChannelType string

const (
	TypeWhatsApp  ChannelType = "whatsapp"
	TypeInstagram ChannelType = "instagram"
)

type OnboardingStatus string

const (
	OnboardingPending         OnboardingStatus = "pending"
	OnboardingSyncingContacts OnboardingStatus = "syncing_contacts"
	OnboardingSyncingHistory  OnboardingStatus = "syncing_history"
	OnboardingCompleted       OnboardingStatus = "completed"
	OnboardingFailed          OnboardingStatus = "failed"
)

// Channel is a workspace messaging channel (WhatsApp or Instagram).
type Channel struct {
	ID              uuid.UUID        `json:"id"`
	TeamID          uuid.UUID        `json:"team_id"`
	Name            string           `json:"name"`
	Provider        string           `json:"provider"`
	ChannelType     ChannelType      `json:"channel_type"`
	Status          Status           `json:"status"`
	ConnectionType  *ConnectionType  `json:"connection_type,omitempty"`
	PhoneNumber     *string          `json:"phone_number,omitempty"`
	WABAID          *string          `json:"waba_id,omitempty"`
	PhoneNumberID   *string          `json:"phone_number_id,omitempty"`
	BusinessID      *string          `json:"business_id,omitempty"`
	IGUserID        *string          `json:"ig_user_id,omitempty"`
	IsCoex          bool             `json:"is_coex"`
	HealthMetadata  []byte           `json:"-"`
	LastConnectedAt *time.Time       `json:"last_connected_at,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	DeletedAt       *time.Time       `json:"deleted_at,omitempty"`
}

type Credentials struct {
	ChannelID       uuid.UUID
	AccessTokenEnc  []byte
	TokenExpiresAt  *time.Time
	UpdatedAt       time.Time
}

type Onboarding struct {
	ChannelID              uuid.UUID
	Status                 OnboardingStatus
	ContactsSyncedCount    int
	HistoryMessagesSynced  int
	HistorySyncProgress    int
	ErrorMessage           *string
	CompletedAt            *time.Time
	UpdatedAt              time.Time
}

type ListFilter struct {
	Status      *Status
	ChannelType *ChannelType
	Search      string
}
