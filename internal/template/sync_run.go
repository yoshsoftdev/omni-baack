package template

import (
	"time"

	"github.com/google/uuid"
)

type SyncRunStatus string

const (
	SyncRunRunning SyncRunStatus = "running"
	SyncRunSuccess SyncRunStatus = "success"
	SyncRunFailed  SyncRunStatus = "failed"
)

// SyncRun tracks a single template synchronization execution.
type SyncRun struct {
	ID           uuid.UUID
	TeamID       uuid.UUID
	ChannelID    uuid.UUID
	StartedAt    time.Time
	CompletedAt  *time.Time
	Status       SyncRunStatus
	SyncedCount  int
	CreatedCount int
	UpdatedCount int
	RemovedCount int64
	ErrorCount   int
	MetaCursor   *string
	ErrorMessage *string
}

// SyncHistoryItem is the API contract for sync history list/detail.
type SyncHistoryItem struct {
	ID          uuid.UUID     `json:"id"`
	ChannelID   uuid.UUID     `json:"channel_id"`
	ChannelName string        `json:"channel_name"`
	Status      SyncRunStatus `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Created     int           `json:"created"`
	Updated     int           `json:"updated"`
	Removed     int64         `json:"removed"`
	ErrorCount  int           `json:"error_count,omitempty"`
}

// SyncMetrics aggregates sync execution stats.
type SyncMetrics struct {
	SyncCount       int64 `json:"sync_count"`
	SuccessCount    int64 `json:"success_count"`
	FailureCount    int64 `json:"failure_count"`
	LastDurationMs  int64 `json:"last_duration_ms"`
	TemplatesSynced int64 `json:"templates_synced"`
}
