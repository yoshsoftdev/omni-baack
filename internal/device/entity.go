package device

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
	ConnectionManual   ConnectionType = "manual"
	ConnectionEmbedded ConnectionType = "embedded_coex"
)

type OnboardingStatus string

const (
	OnboardingPending         OnboardingStatus = "pending"
	OnboardingSyncingContacts OnboardingStatus = "syncing_contacts"
	OnboardingSyncingHistory  OnboardingStatus = "syncing_history"
	OnboardingCompleted       OnboardingStatus = "completed"
	OnboardingFailed          OnboardingStatus = "failed"
)

type Device struct {
	ID                    uuid.UUID
	TeamID                uuid.UUID
	Slug                  *string
	Name                  string
	PhoneNumber           *string
	WabaID                *string
	PhoneNumberID         *string
	AccessToken           *string
	Provider              string
	Status                Status
	ConnectionType        ConnectionType
	IsCoex                bool
	OnboardingStatus      *OnboardingStatus
	ContactsSyncedCount   int
	HistoryMessagesSynced int
	HistorySyncProgress   int
	ContactsSyncRequestID *string
	HistorySyncRequestID  *string
	OnboardingError       *string
	OnboardingCompletedAt *time.Time
	LastConnectedAt       *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

type OnboardingStatusResponse struct {
	Status                *OnboardingStatus `json:"status,omitempty"`
	ContactsSyncedCount   int               `json:"contacts_synced_count"`
	HistoryMessagesSynced int               `json:"history_messages_synced"`
	HistorySyncProgress   int               `json:"history_sync_progress"`
	Error                 *string           `json:"error,omitempty"`
	CompletedAt           *time.Time        `json:"completed_at,omitempty"`
}

type Response struct {
	ID              uuid.UUID                 `json:"id"`
	Slug            *string                   `json:"slug,omitempty"`
	Name            string                    `json:"name"`
	PhoneNumber     *string                   `json:"phone_number,omitempty"`
	WabaID          *string                   `json:"waba_id,omitempty"`
	PhoneNumberID   *string                   `json:"phone_number_id,omitempty"`
	Provider        string                    `json:"provider"`
	Status          Status                    `json:"status"`
	ConnectionType  ConnectionType            `json:"connection_type"`
	IsCoex          bool                      `json:"is_coex"`
	Onboarding      *OnboardingStatusResponse `json:"onboarding,omitempty"`
	LastConnectedAt *time.Time                `json:"last_connected_at,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

func ToResponse(d *Device) Response {
	resp := Response{
		ID:              d.ID,
		Slug:            d.Slug,
		Name:            d.Name,
		PhoneNumber:     d.PhoneNumber,
		WabaID:          d.WabaID,
		PhoneNumberID:   d.PhoneNumberID,
		Provider:        d.Provider,
		Status:          d.Status,
		ConnectionType:  d.ConnectionType,
		IsCoex:          d.IsCoex,
		LastConnectedAt: d.LastConnectedAt,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
	if d.OnboardingStatus != nil || d.IsCoex {
		resp.Onboarding = &OnboardingStatusResponse{
			Status:                d.OnboardingStatus,
			ContactsSyncedCount:   d.ContactsSyncedCount,
			HistoryMessagesSynced: d.HistoryMessagesSynced,
			HistorySyncProgress:   d.HistorySyncProgress,
			Error:                 d.OnboardingError,
			CompletedAt:           d.OnboardingCompletedAt,
		}
	}
	return resp
}
