package operations

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

const (
	defaultBatchLease      = 10 * time.Minute
	defaultRecipientStale  = 15 * time.Minute
)

type PostgresRepository struct {
	q database.Querier
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{q: db.Pool}
}

func (r *PostgresRepository) GetBroadcastStatus(ctx context.Context, batchLease, recipientStale time.Duration) (*BroadcastStatus, error) {
	if batchLease <= 0 {
		batchLease = defaultBatchLease
	}
	if recipientStale <= 0 {
		recipientStale = defaultRecipientStale
	}
	batchStaleBefore := time.Now().UTC().Add(-batchLease)
	recipientStaleBefore := time.Now().UTC().Add(-recipientStale)

	var status BroadcastStatus
	err := r.q.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::int FROM broadcast_jobs
			 WHERE status IN ('pending', 'resolving_audience', 'planning_batches')) AS queued_jobs,
			(SELECT COUNT(*)::int FROM broadcast_batches
			 WHERE status IN ('queued', 'processing')) AS active_batches,
			(SELECT COUNT(*)::int FROM broadcast_batches
			 WHERE status = 'processing' AND locked_at IS NOT NULL AND locked_at < $1) AS stale_batches,
			(
				(SELECT COUNT(*)::int FROM broadcast_batches
				 WHERE status = 'processing' AND locked_at IS NOT NULL AND locked_at < $1)
				+
				(SELECT COUNT(*)::int FROM campaign_recipients
				 WHERE status = 'sending' AND wamid IS NULL AND updated_at < $2)
			) AS recovery_pending
	`, batchStaleBefore, recipientStaleBefore).Scan(
		&status.QueuedJobs,
		&status.ActiveBatches,
		&status.StaleBatches,
		&status.RecoveryPending,
	)
	if err != nil {
		return nil, err
	}
	return &status, nil
}
