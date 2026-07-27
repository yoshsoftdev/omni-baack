package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func (r *PostgresRepository) FindBatchByID(ctx context.Context, batchID uuid.UUID) (*BroadcastBatch, error) {
	return scanBatchRow(r.q.QueryRow(ctx, `
		SELECT id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
			recipients_failed, attempt_count, next_retry_at, locked_by, locked_at, error_summary, created_at, updated_at
		FROM broadcast_batches WHERE id = $1
	`, batchID))
}

func (r *PostgresRepository) ClaimBatchForProcessing(
	ctx context.Context,
	batchID uuid.UUID,
	workerID string,
	staleBefore time.Time,
) (*BroadcastBatch, error) {
	existing, err := r.FindBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if existing.Status == BatchCompleted {
		return existing, nil
	}
	if existing.Status == BatchFailed || existing.Status == BatchCancelled {
		return nil, fmt.Errorf("batch %s is %s", batchID, existing.Status)
	}
	if existing.Status == BatchProcessing && existing.LockedBy != nil && *existing.LockedBy != workerID {
		if existing.LockedAt != nil && existing.LockedAt.After(staleBefore) {
			return nil, ErrBatchInProgress
		}
	}

	row := r.q.QueryRow(ctx, `
		UPDATE broadcast_batches SET
			status = 'processing',
			locked_by = $1,
			locked_at = NOW(),
			updated_at = NOW()
		WHERE id = $2
		  AND status IN ('queued', 'processing')
		  AND (
			status = 'queued'
			OR locked_by = $1
			OR locked_at IS NULL
			OR locked_at < $3
		  )
		RETURNING id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
			recipients_failed, attempt_count, next_retry_at, locked_by, locked_at, error_summary, created_at, updated_at
	`, workerID, batchID, staleBefore)
	batch, err := scanBatchRow(row)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrBatchInProgress
		}
		return nil, err
	}
	return batch, nil
}

func (r *PostgresRepository) RefreshBatchLease(ctx context.Context, batchID uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET locked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'processing'
	`, batchID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CompleteBatch(ctx context.Context, batchID uuid.UUID) error {
	return r.runTx(ctx, func(q database.Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE broadcast_batches SET status = 'completed', locked_by = NULL, locked_at = NULL, updated_at = NOW()
			WHERE id = $1 AND status = 'processing'
		`, batchID)
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() == 0 {
			return repository.ErrNotFound
		}
		_, err = q.Exec(ctx, `
			UPDATE broadcast_jobs
			SET batches_completed = batches_completed + 1,
				status = CASE
					WHEN batches_completed + 1 >= batches_total AND batches_total > 0 THEN 'completed'
					ELSE status
				END,
				completed_at = CASE
					WHEN batches_completed + 1 >= batches_total AND batches_total > 0 THEN NOW()
					ELSE completed_at
				END,
				updated_at = NOW()
			WHERE id = (SELECT job_id FROM broadcast_batches WHERE id = $1)
		`, batchID)
		return repository.MapError(err)
	})
}

func (r *PostgresRepository) RequeueBatch(ctx context.Context, batchID uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET status = 'queued', locked_by = NULL, locked_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'processing'
	`, batchID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkBatchFailed(ctx context.Context, batchID uuid.UUID, summary string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET status = 'failed', error_summary = $1, locked_by = NULL, locked_at = NULL, updated_at = NOW()
		WHERE id = $2 AND status = 'processing'
	`, summary, batchID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListRecipientsByBatch(ctx context.Context, batchID uuid.UUID) ([]Recipient, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, campaign_id, team_id, contact_id, job_id, batch_id,
			provider_profile_id, phone_number, wa_id, variables, status,
			attempt_count, last_error, failure_code, wamid, sent_at, delivered_at, read_at, failed_at, created_at, updated_at
		FROM campaign_recipients
		WHERE batch_id = $1
		ORDER BY created_at ASC
	`, batchID)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]Recipient, 0)
	for rows.Next() {
		rec, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) TransitionCampaignToSending(ctx context.Context, teamID, campaignID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `
		UPDATE campaigns SET status = 'sending', started_at = COALESCE(started_at, NOW()), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL AND status IN ('queued', 'scheduled')
	`, campaignID, teamID)
	return repository.MapError(err)
}

func (r *PostgresRepository) BatchHasActiveRecipients(ctx context.Context, batchID uuid.UUID) (bool, error) {
	var active bool
	err := r.q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM campaign_recipients
			WHERE batch_id = $1 AND status IN ('queued', 'sending')
		)
	`, batchID).Scan(&active)
	if err != nil {
		return false, repository.MapError(err)
	}
	return active, nil
}

func (r *PostgresRepository) IncrementBatchRecipientSent(ctx context.Context, batchID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET recipients_sent = recipients_sent + 1, updated_at = NOW() WHERE id = $1
	`, batchID)
	return repository.MapError(err)
}

func (r *PostgresRepository) IncrementBatchRecipientFailed(ctx context.Context, batchID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET recipients_failed = recipients_failed + 1, updated_at = NOW() WHERE id = $1
	`, batchID)
	return repository.MapError(err)
}

func (r *PostgresRepository) RecoverStaleSendingRecipients(ctx context.Context, staleBefore time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE campaign_recipients SET status = 'queued', updated_at = NOW()
		WHERE id IN (
			SELECT id FROM campaign_recipients
			WHERE status = 'sending'
			  AND wamid IS NULL
			  AND updated_at < $1
			ORDER BY updated_at ASC
			LIMIT $2
		)
	`, staleBefore, limit)
	if err != nil {
		return 0, repository.MapError(err)
	}
	return int(tag.RowsAffected()), nil
}

// ErrBatchInProgress indicates another worker holds a valid lease.
var ErrBatchInProgress = errors.New("batch already processing")

// Ensure PostgresRepository implements SendRepository.
var _ SendRepository = (*PostgresRepository)(nil)
