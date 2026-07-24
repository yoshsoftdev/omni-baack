package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// SendFinalizer atomically finalizes template send outcomes with counters.
type SendFinalizer interface {
	FinalizeRecipientSent(ctx context.Context, in FinalizeSentInput) (bool, error)
	FinalizeRecipientFailed(ctx context.Context, in FinalizeFailedInput) (bool, error)
}

// FinalizeFailedInput carries recipient failure finalization data.
type FinalizeFailedInput struct {
	RecipientID uuid.UUID
	CampaignID  uuid.UUID
	TeamID      uuid.UUID
	From        RecipientStatus
	FailureCode string
	LastError   string
	FailedAt    time.Time
}

// FinalizeSentInput carries recipient send finalization data.
type FinalizeSentInput struct {
	RecipientID      uuid.UUID
	CampaignID       uuid.UUID
	TeamID           uuid.UUID
	From             RecipientStatus
	WAMID            string
	ProviderResponse json.RawMessage
	SentAt           time.Time
}

// FinalizeRecipientSent updates recipient status and counters in one transaction.
// Returns applied=false when the transition was already recorded (idempotent retry).
func (r *PostgresRepository) FinalizeRecipientSent(ctx context.Context, in FinalizeSentInput) (bool, error) {
	if in.From == "" {
		in.From = RecipientSending
	}
	if in.SentAt.IsZero() {
		in.SentAt = time.Now().UTC()
	}

	var applied bool
	err := r.runTx(ctx, func(q database.Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE campaign_recipients SET
				status = 'sent',
				wamid = $1,
				provider_response = COALESCE($2, provider_response),
				sent_at = $3,
				updated_at = NOW()
			WHERE id = $4 AND status = $5::recipient_status
		`, in.WAMID, nullableJSON(in.ProviderResponse), in.SentAt, in.RecipientID, string(in.From))
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() > 0 {
			applied = true
			return applySentCounters(ctx, q, in.CampaignID, in.TeamID, in.SentAt)
		}

		var status string
		var wamid *string
		err = q.QueryRow(ctx, `
			SELECT status, wamid FROM campaign_recipients WHERE id = $1
		`, in.RecipientID).Scan(&status, &wamid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return repository.ErrNotFound
			}
			return repository.MapError(err)
		}
		if RecipientStatus(status) == RecipientSent || RecipientStatus(status) == RecipientDelivered || RecipientStatus(status) == RecipientRead {
			if wamid != nil && *wamid != "" {
				applied = false
				return nil
			}
		}
		return repository.ErrNotFound
	})
	return applied, err
}

func applySentCounters(ctx context.Context, q database.Querier, campaignID, teamID uuid.UUID, sentAt time.Time) error {
	tag, err := q.Exec(ctx, `
		UPDATE campaigns SET sent_count = sent_count + 1, updated_at = NOW() WHERE id = $1
	`, campaignID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	statDate := sentAt.UTC().Truncate(24 * time.Hour)
	_, err = q.Exec(ctx, `
		INSERT INTO campaign_daily_stats (
			team_id, stat_date, campaigns_started, messages_sent, messages_delivered,
			messages_read, messages_failed, replies_received, link_clicks, created_at, updated_at
		) VALUES ($1, $2::date, 0, 1, 0, 0, 0, 0, 0, NOW(), NOW())
		ON CONFLICT (team_id, stat_date) DO UPDATE SET
			messages_sent = campaign_daily_stats.messages_sent + 1,
			updated_at = NOW()
	`, teamID, statDate)
	return repository.MapError(err)
}

// FinalizeRecipientFailed updates recipient status and failure counters in one transaction.
// Returns applied=false when the transition was already recorded (idempotent retry).
func (r *PostgresRepository) FinalizeRecipientFailed(ctx context.Context, in FinalizeFailedInput) (bool, error) {
	if in.From == "" {
		in.From = RecipientSending
	}
	if in.FailedAt.IsZero() {
		in.FailedAt = time.Now().UTC()
	}

	var applied bool
	err := r.runTx(ctx, func(q database.Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE campaign_recipients SET
				status = 'failed',
				failure_code = $1,
				last_error = $2,
				failed_at = $3,
				updated_at = NOW()
			WHERE id = $4 AND status = $5::recipient_status
		`, in.FailureCode, in.LastError, in.FailedAt, in.RecipientID, string(in.From))
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() > 0 {
			applied = true
			return applyFailedCounters(ctx, q, in.CampaignID, in.TeamID, in.FailedAt)
		}

		var status string
		err = q.QueryRow(ctx, `SELECT status FROM campaign_recipients WHERE id = $1`, in.RecipientID).Scan(&status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return repository.ErrNotFound
			}
			return repository.MapError(err)
		}
		if RecipientStatus(status) == RecipientFailed {
			applied = false
			return nil
		}
		return repository.ErrNotFound
	})
	return applied, err
}

func applyFailedCounters(ctx context.Context, q database.Querier, campaignID, teamID uuid.UUID, failedAt time.Time) error {
	tag, err := q.Exec(ctx, `
		UPDATE campaigns SET failed_count = failed_count + 1, updated_at = NOW() WHERE id = $1
	`, campaignID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	statDate := failedAt.UTC().Truncate(24 * time.Hour)
	_, err = q.Exec(ctx, `
		INSERT INTO campaign_daily_stats (
			team_id, stat_date, campaigns_started, messages_sent, messages_delivered,
			messages_read, messages_failed, replies_received, link_clicks, created_at, updated_at
		) VALUES ($1, $2::date, 0, 0, 0, 0, 1, 0, 0, NOW(), NOW())
		ON CONFLICT (team_id, stat_date) DO UPDATE SET
			messages_failed = campaign_daily_stats.messages_failed + 1,
			updated_at = NOW()
	`, teamID, statDate)
	return repository.MapError(err)
}

// Ensure PostgresRepository implements SendFinalizer.
var _ SendFinalizer = (*PostgresRepository)(nil)
