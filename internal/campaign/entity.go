package campaign

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusQueued    Status = "queued"
	StatusSending   Status = "sending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type ContactStatus string

const (
	ContactPending   ContactStatus = "pending"
	ContactSent      ContactStatus = "sent"
	ContactDelivered ContactStatus = "delivered"
	ContactRead      ContactStatus = "read"
	ContactFailed    ContactStatus = "failed"
)

type Campaign struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	DeviceID        uuid.UUID
	TemplateID      uuid.UUID
	Name            string
	Status          Status
	VariableMapping json.RawMessage
	ScheduledAt     *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedBy       uuid.UUID
	TotalContacts   int
	SentCount       int
	DeliveredCount  int
	ReadCount       int
	FailedCount     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type CampaignContact struct {
	ID           uuid.UUID
	CampaignID   uuid.UUID
	ContactID    *uuid.UUID
	Phone        string
	Status       ContactStatus
	Wamid        *string
	ErrorMessage *string
	SentAt       *time.Time
	DeliveredAt  *time.Time
	ReadAt       *time.Time
	CreatedAt    time.Time
}

type Response struct {
	ID              uuid.UUID       `json:"id"`
	DeviceID        uuid.UUID       `json:"device_id"`
	TemplateID      uuid.UUID       `json:"template_id"`
	Name            string          `json:"name"`
	Status          Status          `json:"status"`
	VariableMapping json.RawMessage `json:"variable_mapping"`
	ScheduledAt     *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	TotalContacts   int             `json:"total_contacts"`
	SentCount       int             `json:"sent_count"`
	DeliveredCount  int             `json:"delivered_count"`
	ReadCount       int             `json:"read_count"`
	FailedCount     int             `json:"failed_count"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ReportResponse struct {
	Response
}

func ToResponse(c *Campaign) Response {
	return Response{
		ID:              c.ID,
		DeviceID:        c.DeviceID,
		TemplateID:      c.TemplateID,
		Name:            c.Name,
		Status:          c.Status,
		VariableMapping: c.VariableMapping,
		ScheduledAt:     c.ScheduledAt,
		StartedAt:       c.StartedAt,
		CompletedAt:     c.CompletedAt,
		TotalContacts:   c.TotalContacts,
		SentCount:       c.SentCount,
		DeliveredCount:  c.DeliveredCount,
		ReadCount:       c.ReadCount,
		FailedCount:     c.FailedCount,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

func ToReport(c *Campaign) ReportResponse {
	return ReportResponse{Response: ToResponse(c)}
}
