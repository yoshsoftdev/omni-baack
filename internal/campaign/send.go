package campaign

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SendRepository persists broadcast send worker state.
type SendRepository interface {
	FindBatchByID(ctx context.Context, batchID uuid.UUID) (*BroadcastBatch, error)
	ClaimBatchForProcessing(ctx context.Context, batchID uuid.UUID, workerID string, staleBefore time.Time) (*BroadcastBatch, error)
	RefreshBatchLease(ctx context.Context, batchID uuid.UUID) error
	CompleteBatch(ctx context.Context, batchID uuid.UUID) error
	RequeueBatch(ctx context.Context, batchID uuid.UUID) error
	MarkBatchFailed(ctx context.Context, batchID uuid.UUID, summary string) error
	ListRecipientsByBatch(ctx context.Context, batchID uuid.UUID) ([]Recipient, error)
	TransitionCampaignToSending(ctx context.Context, teamID, campaignID uuid.UUID) error
	BatchHasActiveRecipients(ctx context.Context, batchID uuid.UUID) (bool, error)
	IncrementBatchRecipientSent(ctx context.Context, batchID uuid.UUID) error
	IncrementBatchRecipientFailed(ctx context.Context, batchID uuid.UUID) error
	RecoverStaleSendingRecipients(ctx context.Context, staleBefore time.Time, limit int) (int, error)
}
