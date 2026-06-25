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

type SendMode string

const (
	SendNow   SendMode = "now"
	SendLater SendMode = "later"
)

type AudienceSourceType string

const (
	AudienceSegment AudienceSourceType = "segment"
	AudienceTag     AudienceSourceType = "tag"
	AudienceContact AudienceSourceType = "contact"
)

type Campaign struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	ChannelID       uuid.UUID
	TemplateID      uuid.UUID
	SegmentID       *uuid.UUID
	Name            string
	Description     *string
	Status          Status
	VariableMapping json.RawMessage
	SampleValues    json.RawMessage
	Timezone        *string
	SendMode        *SendMode
	ScheduledAt     *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	TotalContacts   int
	SentCount       int
	DeliveredCount  int
	ReadCount       int
	FailedCount     int
	ReplyCount      int
	ClickedCount    int
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Audience struct {
	ID         uuid.UUID
	CampaignID uuid.UUID
	SourceType AudienceSourceType
	SourceID   uuid.UUID
	CreatedAt  time.Time
}

type BroadcastJobStatus string

const (
	JobPending           BroadcastJobStatus = "pending"
	JobResolvingAudience BroadcastJobStatus = "resolving_audience"
	JobPlanningBatches   BroadcastJobStatus = "planning_batches"
	JobSending           BroadcastJobStatus = "sending"
	JobCompleted         BroadcastJobStatus = "completed"
	JobFailed            BroadcastJobStatus = "failed"
	JobCancelled         BroadcastJobStatus = "cancelled"
)

type BroadcastJob struct {
	ID                uuid.UUID
	CampaignID        uuid.UUID
	TeamID            uuid.UUID
	ChannelID         uuid.UUID
	Status            BroadcastJobStatus
	TotalRecipients   int
	BatchesTotal      int
	BatchesCompleted  int
	RecipientsSent    int
	StartedAt         *time.Time
	CompletedAt       *time.Time
	CancelledAt       *time.Time
	CancelReason      *string
	ErrorSummary      *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type BroadcastBatchStatus string

const (
	BatchPending    BroadcastBatchStatus = "pending"
	BatchQueued     BroadcastBatchStatus = "queued"
	BatchProcessing BroadcastBatchStatus = "processing"
	BatchCompleted  BroadcastBatchStatus = "completed"
	BatchFailed     BroadcastBatchStatus = "failed"
	BatchCancelled  BroadcastBatchStatus = "cancelled"
)

type BroadcastBatch struct {
	ID               uuid.UUID
	JobID            uuid.UUID
	CampaignID       uuid.UUID
	BatchNumber      int
	Status           BroadcastBatchStatus
	RecipientCount   int
	RecipientsSent   int
	RecipientsFailed int
	AttemptCount     int
	NextRetryAt      *time.Time
	LockedBy         *string
	LockedAt         *time.Time
	ErrorSummary     *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type RecipientStatus string

const (
	RecipientPending   RecipientStatus = "pending"
	RecipientQueued    RecipientStatus = "queued"
	RecipientSending   RecipientStatus = "sending"
	RecipientSent      RecipientStatus = "sent"
	RecipientDelivered RecipientStatus = "delivered"
	RecipientRead      RecipientStatus = "read"
	RecipientFailed    RecipientStatus = "failed"
	RecipientCancelled RecipientStatus = "cancelled"
	RecipientSkipped   RecipientStatus = "skipped"
)

type Recipient struct {
	ID           uuid.UUID
	CampaignID   uuid.UUID
	TeamID       uuid.UUID
	ContactID    uuid.UUID
	JobID        *uuid.UUID
	BatchID      *uuid.UUID
	Phone        string
	Status       RecipientStatus
	AttemptCount int
	LastError    *string
	FailureCode  *string
	WAMID        *string
	SentAt       *time.Time
	DeliveredAt  *time.Time
	ReadAt       *time.Time
	FailedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AudienceSnapshot struct {
	ID              uuid.UUID
	CampaignID      uuid.UUID
	JobID           uuid.UUID
	SnapshotAt      time.Time
	SourceSummary   json.RawMessage
	ResolvedCount   int
	SkippedOptOut   int
	SkippedInvalid  int
	CreatedAt       time.Time
}

type ListFilter struct {
	Status    *Status
	ChannelID *uuid.UUID
	Search    string
}

type CounterDelta struct {
	Sent      int
	Delivered int
	Read      int
	Failed    int
}

// RecipientDeliveryUpdate carries optional fields applied on status transition.
type RecipientDeliveryUpdate struct {
	WAMID            *string
	LastError        *string
	FailureCode      *string
	ProviderResponse json.RawMessage
	IncrementAttempt bool
}

// RecipientStatusCounts aggregates recipient rows for reconciliation.
type RecipientStatusCounts struct {
	Sent      int
	Delivered int
	Read      int
	Failed    int
}
