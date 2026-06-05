package broadcast

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusSending   Status = "sending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Broadcast struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	DeviceID        uuid.UUID
	Name            string
	Message         string
	Status          Status
	ScheduledAt     *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedBy       uuid.UUID
	TotalRecipients int
	SentCount       int
	FailedCount     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Recipient struct {
	ID           uuid.UUID
	BroadcastID  uuid.UUID
	ContactID    *uuid.UUID
	Phone        string
	Status       string
	WAMID        *string
	ErrorMessage *string
	SentAt       *time.Time
	CreatedAt    time.Time
}

type Response struct {
	ID              uuid.UUID  `json:"id"`
	DeviceID        uuid.UUID  `json:"device_id"`
	Name            string     `json:"name"`
	Message         string     `json:"message"`
	Status          Status     `json:"status"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	TotalRecipients int        `json:"total_recipients"`
	SentCount       int        `json:"sent_count"`
	FailedCount     int        `json:"failed_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func ToResponse(b *Broadcast) Response {
	return Response{
		ID:              b.ID,
		DeviceID:        b.DeviceID,
		Name:            b.Name,
		Message:         b.Message,
		Status:          b.Status,
		ScheduledAt:     b.ScheduledAt,
		StartedAt:       b.StartedAt,
		CompletedAt:     b.CompletedAt,
		TotalRecipients: b.TotalRecipients,
		SentCount:       b.SentCount,
		FailedCount:     b.FailedCount,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}
}
