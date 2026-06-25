package operations

import (
	"context"
	"time"
)

// BroadcastStatus summarizes broadcast pipeline health (read-only).
type BroadcastStatus struct {
	QueuedJobs      int `json:"queued_jobs"`
	ActiveBatches   int `json:"active_batches"`
	StaleBatches    int `json:"stale_batches"`
	RecoveryPending int `json:"recovery_pending"`
}

// WorkerStatus summarizes registered background workers (read-only).
type WorkerStatus struct {
	Orchestrators  int `json:"orchestrators"`
	SendWorkers    int `json:"send_workers"`
	EventConsumers int `json:"event_consumers"`
}

// Repository loads operational broadcast metrics.
type Repository interface {
	GetBroadcastStatus(ctx context.Context, batchLease, recipientStale time.Duration) (*BroadcastStatus, error)
}

// WorkerCounter counts live worker registrations.
type WorkerCounter interface {
	CountOrchestrators(ctx context.Context) (int, error)
	CountSendWorkers(ctx context.Context) (int, error)
	CountEventConsumers(ctx context.Context) (int, error)
}
