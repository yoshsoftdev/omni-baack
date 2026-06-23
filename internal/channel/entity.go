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
)

type OnboardingStatus string

const (
	OnboardingPending         OnboardingStatus = "pending"
	OnboardingSyncingContacts OnboardingStatus = "syncing_contacts"
	OnboardingSyncingHistory  OnboardingStatus = "syncing_history"
	OnboardingCompleted       OnboardingStatus = "completed"
	OnboardingFailed          OnboardingStatus = "failed"
)

// Channel is a workspace WhatsApp channel (API: ChannelDTO).
type Channel struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	Name            string
	Provider        string
	Status          Status
	ConnectionType  *ConnectionType
	PhoneNumber     *string
	WABAID          *string
	PhoneNumberID   *string
	IsCoex          bool
	LastConnectedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
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
	Status *Status
	Search string
}
