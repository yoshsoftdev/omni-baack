package campaign

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func (r *PostgresRepository) ClaimDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.q.Query(ctx, `
		SELECT id, team_id, channel_id, template_id, segment_id, name, description, status,
			variable_mapping, sample_values, timezone, send_mode, scheduled_at, started_at, completed_at,
			total_contacts, sent_count, delivered_count, read_count, failed_count, reply_count, clicked_count,
			created_by, created_at, updated_at, deleted_at
		FROM campaigns
		WHERE deleted_at IS NULL
		  AND status = 'scheduled'
		  AND scheduled_at IS NOT NULL
		  AND scheduled_at <= $1
		ORDER BY scheduled_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, now, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]Campaign, 0)
	for rows.Next() {
		c, err := scanCampaignRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) TransitionCampaignToQueued(ctx context.Context, teamID, campaignID uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE campaigns SET status = 'queued', updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL AND status = 'scheduled'
	`, campaignID, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListPendingJobs(ctx context.Context, limit int) ([]BroadcastJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.q.Query(ctx, `
		SELECT id, campaign_id, team_id, channel_id, status, total_recipients, batches_total, batches_completed,
			recipients_sent, started_at, completed_at, cancelled_at, cancel_reason, error_summary, created_at, updated_at
		FROM broadcast_jobs
		WHERE status IN ('pending', 'resolving_audience', 'planning_batches')
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]BroadcastJob, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) FindJobByID(ctx context.Context, jobID uuid.UUID) (*BroadcastJob, error) {
	return scanJob(r.q.QueryRow(ctx, `
		SELECT id, campaign_id, team_id, channel_id, status, total_recipients, batches_total, batches_completed,
			recipients_sent, started_at, completed_at, cancelled_at, cancel_reason, error_summary, created_at, updated_at
		FROM broadcast_jobs WHERE id = $1
	`, jobID))
}

func (r *PostgresRepository) CountRecipientsByJob(ctx context.Context, jobID uuid.UUID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT COUNT(*)::int FROM campaign_recipients WHERE job_id = $1`, jobID).Scan(&n)
	return n, repository.MapError(err)
}

func (r *PostgresRepository) CountBatchesByJob(ctx context.Context, jobID uuid.UUID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT COUNT(*)::int FROM broadcast_batches WHERE job_id = $1`, jobID).Scan(&n)
	return n, repository.MapError(err)
}

func (r *PostgresRepository) UpdateCampaignTotalContacts(ctx context.Context, campaignID uuid.UUID, total int) error {
	_, err := r.q.Exec(ctx, `
		UPDATE campaigns SET total_contacts = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL
	`, total, campaignID)
	return repository.MapError(err)
}

func (r *PostgresRepository) PlanBatchesForJob(ctx context.Context, jobID uuid.UUID, batchSize int) ([]BroadcastBatch, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	var batches []BroadcastBatch
	err := r.runTx(ctx, func(q database.Querier) error {
		tx, ok := q.(pgx.Tx)
		if !ok {
			return fmt.Errorf("plan batches requires transaction")
		}

		var existing int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM broadcast_batches WHERE job_id = $1`, jobID).Scan(&existing); err != nil {
			return repository.MapError(err)
		}
		if existing > 0 {
			rows, err := tx.Query(ctx, `
				SELECT id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
					recipients_failed, attempt_count, next_retry_at, locked_by, locked_at, error_summary, created_at, updated_at
				FROM broadcast_batches WHERE job_id = $1 ORDER BY batch_number ASC
			`, jobID)
			if err != nil {
				return repository.MapError(err)
			}
			defer rows.Close()
			for rows.Next() {
				b, err := scanBatchRow(rows)
				if err != nil {
					return err
				}
				batches = append(batches, *b)
			}
			return rows.Err()
		}

		var job BroadcastJob
		var jobStatus string
		if err := tx.QueryRow(ctx, `
			SELECT id, campaign_id, team_id, channel_id, status, total_recipients, batches_total, batches_completed,
				recipients_sent, started_at, completed_at, cancelled_at, cancel_reason, error_summary, created_at, updated_at
			FROM broadcast_jobs WHERE id = $1 FOR UPDATE
		`, jobID).Scan(
			&job.ID, &job.CampaignID, &job.TeamID, &job.ChannelID, &jobStatus, &job.TotalRecipients,
			&job.BatchesTotal, &job.BatchesCompleted, &job.RecipientsSent, &job.StartedAt, &job.CompletedAt,
			&job.CancelledAt, &job.CancelReason, &job.ErrorSummary, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return repository.MapError(err)
		}
		job.Status = BroadcastJobStatus(jobStatus)

		rows, err := tx.Query(ctx, `
			SELECT id FROM campaign_recipients
			WHERE job_id = $1 AND status = 'pending'
			ORDER BY created_at ASC, id ASC
		`, jobID)
		if err != nil {
			return repository.MapError(err)
		}
		var recipientIDs []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			recipientIDs = append(recipientIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(recipientIDs) == 0 {
			return nil
		}

		now := time.Now().UTC()
		batchNum := 0
		for i := 0; i < len(recipientIDs); i += batchSize {
			batchNum++
			end := i + batchSize
			if end > len(recipientIDs) {
				end = len(recipientIDs)
			}
			chunk := recipientIDs[i:end]
			batchID := uuid.New()
			b := BroadcastBatch{
				ID:             batchID,
				JobID:          jobID,
				CampaignID:     job.CampaignID,
				BatchNumber:    batchNum,
				Status:         BatchPending,
				RecipientCount: len(chunk),
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO broadcast_batches (
					id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
					recipients_failed, attempt_count, created_at, updated_at
				) VALUES ($1,$2,$3,$4,$5,$6,0,0,0,$7,$7)
			`, b.ID, b.JobID, b.CampaignID, b.BatchNumber, string(b.Status), b.RecipientCount, now); err != nil {
				return repository.MapError(err)
			}
			tag, err := tx.Exec(ctx, `
				UPDATE campaign_recipients SET batch_id = $1, status = 'queued', updated_at = NOW()
				WHERE id = ANY($2) AND job_id = $3 AND status = 'pending'
			`, batchID, chunk, jobID)
			if err != nil {
				return repository.MapError(err)
			}
			if tag.RowsAffected() != int64(len(chunk)) {
				return fmt.Errorf("batch assign affected %d of %d recipients", tag.RowsAffected(), len(chunk))
			}
			batches = append(batches, b)
		}

		job.BatchesTotal = batchNum
		job.Status = JobSending
		job.UpdatedAt = now
		if _, err := tx.Exec(ctx, `
			UPDATE broadcast_jobs SET status=$1, batches_total=$2, updated_at=$3 WHERE id=$4
		`, string(job.Status), job.BatchesTotal, now, job.ID); err != nil {
			return repository.MapError(err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE campaigns SET status='sending', started_at=COALESCE(started_at, $1), updated_at=$1 WHERE id=$2
		`, now, job.CampaignID); err != nil {
			return repository.MapError(err)
		}
		return nil
	})
	return batches, err
}

func (r *PostgresRepository) MarkBatchesQueued(ctx context.Context, batchIDs []uuid.UUID) error {
	if len(batchIDs) == 0 {
		return nil
	}
	_, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET status = 'queued', updated_at = NOW()
		WHERE id = ANY($1) AND status = 'pending'
	`, batchIDs)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListStaleProcessingBatches(ctx context.Context, leaseExpiredBefore time.Time, limit int) ([]BroadcastBatch, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.Query(ctx, `
		SELECT id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
			recipients_failed, attempt_count, next_retry_at, locked_by, locked_at, error_summary, created_at, updated_at
		FROM broadcast_batches
		WHERE status = 'processing'
		  AND locked_at IS NOT NULL
		  AND locked_at < $1
		ORDER BY locked_at ASC
		LIMIT $2
	`, leaseExpiredBefore, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]BroadcastBatch, 0)
	for rows.Next() {
		b, err := scanBatchRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RequeueStaleBatch(ctx context.Context, batchID uuid.UUID) error {
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

func (r *PostgresRepository) ResetSendingRecipientsWithoutWAMID(ctx context.Context, batchID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `
		UPDATE campaign_recipients SET status = 'queued', updated_at = NOW()
		WHERE batch_id = $1 AND status = 'sending' AND wamid IS NULL
	`, batchID)
	return repository.MapError(err)
}

func (r *PostgresRepository) InsertOutbox(ctx context.Context, subject string, payload []byte) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO broadcast_outbox (id, subject, payload, created_at) VALUES ($1, $2, $3, NOW())
	`, uuid.New(), subject, payload)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.Query(ctx, `
		SELECT id, subject, payload, created_at FROM broadcast_outbox
		WHERE published_at IS NULL ORDER BY created_at ASC LIMIT $1
	`, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]OutboxMessage, 0)
	for rows.Next() {
		var m OutboxMessage
		if err := rows.Scan(&m.ID, &m.Subject, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MarkOutboxPublished(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE broadcast_outbox SET published_at = NOW() WHERE id = $1 AND published_at IS NULL
	`, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// Ensure PostgresRepository implements OrchestrationRepository.
var _ OrchestrationRepository = (*PostgresRepository)(nil)