package campaign

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// TransitionCampaignToQueuedForSend moves draft or scheduled campaigns into the orchestration queue.
func (r *PostgresRepository) TransitionCampaignToQueuedForSend(ctx context.Context, teamID, campaignID uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE campaigns SET status = 'queued', updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL AND status IN ('draft', 'scheduled')
	`, campaignID, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// CancelCampaignExecution cancels a campaign, its active job, batches, and cancellable recipients atomically.
func (r *PostgresRepository) CancelCampaignExecution(ctx context.Context, teamID, campaignID uuid.UUID, reason string) error {
	now := time.Now().UTC()
	return r.runTx(ctx, func(q database.Querier) error {
		var status string
		err := q.QueryRow(ctx, `
			SELECT status FROM campaigns
			WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
			FOR UPDATE
		`, campaignID, teamID).Scan(&status)
		if err != nil {
			return repository.MapError(err)
		}
		current := Status(status)
		if current == StatusCancelled {
			return nil
		}
		if !CanCancelCampaign(current) {
			return repository.ErrConflict
		}

		tag, err := q.Exec(ctx, `
			UPDATE campaigns SET status = 'cancelled', updated_at = NOW()
			WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
		`, campaignID, teamID)
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() == 0 {
			return repository.ErrNotFound
		}

		_, err = q.Exec(ctx, `
			UPDATE campaign_recipients SET status = 'cancelled', updated_at = NOW()
			WHERE campaign_id = $1
			  AND (
			    status IN ('pending', 'queued')
			    OR (status = 'sending' AND wamid IS NULL)
			  )
		`, campaignID)
		if err != nil {
			return repository.MapError(err)
		}

		_, err = q.Exec(ctx, `
			UPDATE broadcast_batches SET status = 'cancelled', updated_at = NOW()
			WHERE campaign_id = $1 AND status IN ('pending', 'queued', 'processing')
		`, campaignID)
		if err != nil {
			return repository.MapError(err)
		}

		_, err = q.Exec(ctx, `
			UPDATE broadcast_jobs SET
				status = 'cancelled',
				cancelled_at = $1,
				cancel_reason = $2,
				updated_at = NOW()
			WHERE campaign_id = $3 AND status NOT IN ('completed', 'failed', 'cancelled')
		`, now, nullableString(reason), campaignID)
		return repository.MapError(err)
	})
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
