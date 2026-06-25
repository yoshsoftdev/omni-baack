package campaign

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OutboxMessage is a pending broadcast NATS publish.
type OutboxMessage struct {
	ID        uuid.UUID
	Subject   string
	Payload   []byte
	CreatedAt time.Time
}

// OrchestrationRepository extends campaign persistence for broadcast orchestration.
type OrchestrationRepository interface {
	ClaimDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error)
	TransitionCampaignToQueued(ctx context.Context, teamID, campaignID uuid.UUID) error
	ListPendingJobs(ctx context.Context, limit int) ([]BroadcastJob, error)
	FindJobByID(ctx context.Context, jobID uuid.UUID) (*BroadcastJob, error)
	CountRecipientsByJob(ctx context.Context, jobID uuid.UUID) (int, error)
	CountBatchesByJob(ctx context.Context, jobID uuid.UUID) (int, error)
	UpdateCampaignTotalContacts(ctx context.Context, campaignID uuid.UUID, total int) error
	PlanBatchesForJob(ctx context.Context, jobID uuid.UUID, batchSize int) ([]BroadcastBatch, error)
	MarkBatchesQueued(ctx context.Context, batchIDs []uuid.UUID) error
	ListStaleProcessingBatches(ctx context.Context, leaseExpiredBefore time.Time, limit int) ([]BroadcastBatch, error)
	RequeueStaleBatch(ctx context.Context, batchID uuid.UUID) error
	ResetSendingRecipientsWithoutWAMID(ctx context.Context, batchID uuid.UUID) error

	InsertOutbox(ctx context.Context, subject string, payload []byte) error
	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error)
	MarkOutboxPublished(ctx context.Context, id uuid.UUID) error
}
