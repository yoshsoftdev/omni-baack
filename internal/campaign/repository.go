package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (CampaignRepository) persists campaigns and broadcast execution data.
type Repository interface {
	Create(ctx context.Context, c *Campaign, audiences []Audience) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Campaign, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Campaign], error)
	Update(ctx context.Context, c *Campaign) error
	UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	ListAudiences(ctx context.Context, campaignID uuid.UUID) ([]Audience, error)
	LoadListEnrichment(ctx context.Context, teamID uuid.UUID, campaigns []Campaign) (map[uuid.UUID]ListEnrichment, error)

	CreateJob(ctx context.Context, job *BroadcastJob) error
	UpdateJob(ctx context.Context, job *BroadcastJob) error
	FindActiveJobByCampaign(ctx context.Context, campaignID uuid.UUID) (*BroadcastJob, error)

	CreateBatch(ctx context.Context, batch *BroadcastBatch) error
	UpdateBatch(ctx context.Context, batch *BroadcastBatch) error
	ListBatchesByJob(ctx context.Context, jobID uuid.UUID) ([]BroadcastBatch, error)

	CreateAudienceSnapshot(ctx context.Context, snap *AudienceSnapshot) error
	BulkCreateRecipients(ctx context.Context, recipients []Recipient, snapshotAt time.Time) error
	UpdateRecipientStatus(ctx context.Context, id uuid.UUID, from, to RecipientStatus, wamid *string, lastError *string) error
	UpdateRecipientDelivery(ctx context.Context, id uuid.UUID, from, to RecipientStatus, update RecipientDeliveryUpdate) error
	FindRecipientByWAMID(ctx context.Context, wamid string) (*Recipient, error)
	FindRecipientByWAMIDForTeam(ctx context.Context, teamID uuid.UUID, wamid string) (*Recipient, error)
	ApplyCounterDelta(ctx context.Context, campaignID uuid.UUID, delta CounterDelta) error
	CountNonTerminalRecipients(ctx context.Context, campaignID uuid.UUID) (int, error)
	CompleteCampaign(ctx context.Context, teamID, campaignID uuid.UUID, completedAt time.Time) error
	AggregateRecipientStatusCounts(ctx context.Context, campaignID uuid.UUID) (RecipientStatusCounts, error)
	SetCampaignCounters(ctx context.Context, campaignID uuid.UUID, counts RecipientStatusCounts) error
	RepairCampaignCounters(ctx context.Context, campaignID uuid.UUID) (bool, error)
	ListCampaignIDsForReconciliation(ctx context.Context, updatedSince time.Time) ([]uuid.UUID, error)

	TransitionCampaignToQueuedForSend(ctx context.Context, teamID, campaignID uuid.UUID) error
	CancelCampaignExecution(ctx context.Context, teamID, campaignID uuid.UUID, reason string) error
}

type PostgresRepository struct {
	q    database.Querier
	db   *database.DB
	inTx bool
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{q: db.Pool, db: db, inTx: false}
}

// NewRepositoryWithQuerier returns a repository bound to a pool or transaction querier.
func NewRepositoryWithQuerier(q database.Querier) *PostgresRepository {
	return &PostgresRepository{q: q, inTx: database.IsTx(q)}
}

func (r *PostgresRepository) runTx(ctx context.Context, fn func(database.Querier) error) error {
	if r.inTx {
		return fn(r.q)
	}
	if r.db == nil {
		return repository.ErrNestedTransaction
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		return fn(tx)
	})
}

func (r *PostgresRepository) Create(ctx context.Context, c *Campaign, audiences []Audience) error {
	return r.runTx(ctx, func(q database.Querier) error {
		_, err := q.Exec(ctx, `
			INSERT INTO campaigns (
				id, team_id, channel_id, template_id, segment_id, name, description, status,
				variable_mapping, sample_values, timezone, send_mode, scheduled_at,
				total_contacts, created_by, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		`, c.ID, c.TeamID, c.ChannelID, c.TemplateID, c.SegmentID, c.Name, c.Description, string(c.Status),
			c.VariableMapping, c.SampleValues, c.Timezone, c.SendMode, c.ScheduledAt,
			c.TotalContacts, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
		if err != nil {
			return repository.MapError(err)
		}
		for _, a := range audiences {
			_, err := q.Exec(ctx,
				`INSERT INTO campaign_audiences (id, campaign_id, source_type, source_id, created_at) VALUES ($1,$2,$3,$4,$5)`,
				a.ID, c.ID, string(a.SourceType), a.SourceID, a.CreatedAt,
			)
			if err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Campaign, error) {
	return scanCampaign(r.q.QueryRow(ctx, `
		SELECT id, team_id, channel_id, template_id, segment_id, name, description, status,
			variable_mapping, sample_values, timezone, send_mode, scheduled_at, started_at, completed_at,
			total_contacts, sent_count, delivered_count, read_count, failed_count, reply_count, clicked_count,
			created_by, created_at, updated_at, deleted_at
		FROM campaigns WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Campaign], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(` AND channel_id = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}
	var total int64
	if err := r.q.QueryRow(ctx, `SELECT COUNT(*) FROM campaigns WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Campaign]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.q.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, channel_id, template_id, segment_id, name, description, status,
			variable_mapping, sample_values, timezone, send_mode, scheduled_at, started_at, completed_at,
			total_contacts, sent_count, delivered_count, read_count, failed_count, reply_count, clicked_count,
			created_by, created_at, updated_at, deleted_at
		FROM campaigns WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Campaign]{}, err
	}
	defer rows.Close()
	items := make([]Campaign, 0)
	for rows.Next() {
		c, err := scanCampaignRow(rows)
		if err != nil {
			return repository.Page[Campaign]{}, err
		}
		items = append(items, *c)
	}
	return repository.Page[Campaign]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, c *Campaign) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE campaigns SET name=$1, description=$2, variable_mapping=$3, sample_values=$4,
			timezone=$5, send_mode=$6, scheduled_at=$7, segment_id=$8, updated_at=$9
		WHERE id=$10 AND team_id=$11 AND deleted_at IS NULL AND status IN ('draft','scheduled')
	`, c.Name, c.Description, c.VariableMapping, c.SampleValues, c.Timezone, c.SendMode, c.ScheduledAt,
		c.SegmentID, c.UpdatedAt, c.ID, c.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE campaigns SET status = $1, updated_at = NOW() WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL`,
		string(status), id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE campaigns SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL AND status = 'draft'`,
		id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListAudiences(ctx context.Context, campaignID uuid.UUID) ([]Audience, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, campaign_id, source_type, source_id, created_at FROM campaign_audiences WHERE campaign_id = $1
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Audience, 0)
	for rows.Next() {
		var a Audience
		var st string
		if err := rows.Scan(&a.ID, &a.CampaignID, &st, &a.SourceID, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.SourceType = AudienceSourceType(st)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateJob(ctx context.Context, job *BroadcastJob) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO broadcast_jobs (
			id, campaign_id, team_id, channel_id, status, total_recipients,
			batches_total, batches_completed, recipients_sent, started_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, job.ID, job.CampaignID, job.TeamID, job.ChannelID, string(job.Status), job.TotalRecipients,
		job.BatchesTotal, job.BatchesCompleted, job.RecipientsSent, job.StartedAt, job.CreatedAt, job.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) UpdateJob(ctx context.Context, job *BroadcastJob) error {
	_, err := r.q.Exec(ctx, `
		UPDATE broadcast_jobs SET status=$1, total_recipients=$2, batches_total=$3, batches_completed=$4,
			recipients_sent=$5, started_at=$6, completed_at=$7, cancelled_at=$8, cancel_reason=$9,
			error_summary=$10, updated_at=$11 WHERE id=$12
	`, string(job.Status), job.TotalRecipients, job.BatchesTotal, job.BatchesCompleted,
		job.RecipientsSent, job.StartedAt, job.CompletedAt, job.CancelledAt, job.CancelReason,
		job.ErrorSummary, job.UpdatedAt, job.ID)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindActiveJobByCampaign(ctx context.Context, campaignID uuid.UUID) (*BroadcastJob, error) {
	return scanJob(r.q.QueryRow(ctx, `
		SELECT id, campaign_id, team_id, channel_id, status, total_recipients, batches_total, batches_completed,
			recipients_sent, started_at, completed_at, cancelled_at, cancel_reason, error_summary, created_at, updated_at
		FROM broadcast_jobs WHERE campaign_id = $1 AND status NOT IN ('completed','failed','cancelled')
		ORDER BY created_at DESC LIMIT 1
	`, campaignID))
}

func (r *PostgresRepository) CreateBatch(ctx context.Context, batch *BroadcastBatch) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO broadcast_batches (
			id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
			recipients_failed, attempt_count, next_retry_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, batch.ID, batch.JobID, batch.CampaignID, batch.BatchNumber, string(batch.Status),
		batch.RecipientCount, batch.RecipientsSent, batch.RecipientsFailed, batch.AttemptCount,
		batch.NextRetryAt, batch.CreatedAt, batch.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) UpdateBatch(ctx context.Context, batch *BroadcastBatch) error {
	_, err := r.q.Exec(ctx, `
		UPDATE broadcast_batches SET status=$1, recipients_sent=$2, recipients_failed=$3, attempt_count=$4,
			next_retry_at=$5, locked_by=$6, locked_at=$7, error_summary=$8, updated_at=$9 WHERE id=$10
	`, string(batch.Status), batch.RecipientsSent, batch.RecipientsFailed, batch.AttemptCount,
		batch.NextRetryAt, batch.LockedBy, batch.LockedAt, batch.ErrorSummary, batch.UpdatedAt, batch.ID)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListBatchesByJob(ctx context.Context, jobID uuid.UUID) ([]BroadcastBatch, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, job_id, campaign_id, batch_number, status, recipient_count, recipients_sent,
			recipients_failed, attempt_count, next_retry_at, locked_by, locked_at, error_summary, created_at, updated_at
		FROM broadcast_batches WHERE job_id = $1 ORDER BY batch_number ASC
	`, jobID)
	if err != nil {
		return nil, err
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

func (r *PostgresRepository) CreateAudienceSnapshot(ctx context.Context, snap *AudienceSnapshot) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO campaign_audience_snapshots (
			id, campaign_id, job_id, snapshot_at, source_summary, resolved_count, skipped_opt_out, skipped_invalid, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, snap.ID, snap.CampaignID, snap.JobID, snap.SnapshotAt, snap.SourceSummary,
		snap.ResolvedCount, snap.SkippedOptOut, snap.SkippedInvalid, snap.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) BulkCreateRecipients(ctx context.Context, recipients []Recipient, snapshotAt time.Time) error {
	if len(recipients) == 0 {
		return nil
	}
	return r.runTx(ctx, func(q database.Querier) error {
		tx, ok := q.(pgx.Tx)
		if !ok {
			return fmt.Errorf("bulk create recipients requires pgx.Tx")
		}
		batch := &pgx.Batch{}
		for _, rec := range recipients {
			vars := rec.Variables
			if len(vars) == 0 {
				vars = json.RawMessage(`{}`)
			}
			batch.Queue(`
				INSERT INTO campaign_recipients (
					id, campaign_id, team_id, contact_id, job_id, batch_id,
					provider_profile_id, phone_number, wa_id, variables,
					status, attempt_count, last_error, created_at, updated_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			`, rec.ID, rec.CampaignID, rec.TeamID, rec.ContactID, rec.JobID, rec.BatchID,
				rec.ProviderProfileID, rec.PhoneNumber, rec.WAID, vars,
				string(rec.Status), rec.AttemptCount, rec.LastError, snapshotAt, snapshotAt)
		}
		br := tx.SendBatch(ctx, batch)
		defer br.Close()
		for range recipients {
			if _, err := br.Exec(); err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) UpdateRecipientStatus(ctx context.Context, id uuid.UUID, from, to RecipientStatus, wamid *string, lastError *string) error {
	return r.UpdateRecipientDelivery(ctx, id, from, to, RecipientDeliveryUpdate{
		WAMID:     wamid,
		LastError: lastError,
	})
}

func (r *PostgresRepository) UpdateRecipientDelivery(ctx context.Context, id uuid.UUID, from, to RecipientStatus, update RecipientDeliveryUpdate) error {
	// Cast $1/$8 to recipient_status explicitly. Comparing the same param to text
	// literals (e.g. $1 = 'sent') while also assigning to an enum column causes
	// PostgreSQL SQLSTATE 42P08 (inconsistent types deduced for parameter).
	tag, err := r.q.Exec(ctx, `
		UPDATE campaign_recipients SET status = $1::recipient_status, wamid = COALESCE($2, wamid),
			last_error = COALESCE($3, last_error), failure_code = COALESCE($4, failure_code),
			provider_response = COALESCE($5, provider_response),
			attempt_count = CASE WHEN $6 THEN attempt_count + 1 ELSE attempt_count END,
			sent_at = CASE WHEN $1::recipient_status = 'sent'::recipient_status THEN NOW() ELSE sent_at END,
			delivered_at = CASE WHEN $1::recipient_status = 'delivered'::recipient_status THEN NOW() ELSE delivered_at END,
			read_at = CASE WHEN $1::recipient_status = 'read'::recipient_status THEN NOW() ELSE read_at END,
			failed_at = CASE WHEN $1::recipient_status = 'failed'::recipient_status THEN NOW() ELSE failed_at END,
			updated_at = NOW()
		WHERE id = $7 AND status = $8::recipient_status
	`, string(to), update.WAMID, update.LastError, update.FailureCode, nullableJSON(update.ProviderResponse),
		update.IncrementAttempt, id, string(from))
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) FindRecipientByWAMID(ctx context.Context, wamid string) (*Recipient, error) {
	return scanRecipient(r.q.QueryRow(ctx, `
		SELECT id, campaign_id, team_id, contact_id, job_id, batch_id,
			provider_profile_id, phone_number, wa_id, variables, status,
			attempt_count, last_error, failure_code, wamid, sent_at, delivered_at, read_at, failed_at, created_at, updated_at
		FROM campaign_recipients WHERE wamid = $1 LIMIT 1
	`, wamid))
}

func (r *PostgresRepository) FindRecipientByWAMIDForTeam(ctx context.Context, teamID uuid.UUID, wamid string) (*Recipient, error) {
	return scanRecipient(r.q.QueryRow(ctx, `
		SELECT id, campaign_id, team_id, contact_id, job_id, batch_id,
			provider_profile_id, phone_number, wa_id, variables, status,
			attempt_count, last_error, failure_code, wamid, sent_at, delivered_at, read_at, failed_at, created_at, updated_at
		FROM campaign_recipients WHERE wamid = $1 AND team_id = $2 LIMIT 1
	`, wamid, teamID))
}

func (r *PostgresRepository) CountNonTerminalRecipients(ctx context.Context, campaignID uuid.UUID) (int, error) {
	var count int
	// Campaign send completion: sent/delivered are terminal for lifecycle (do not wait for read).
	// Delivery/read counters may still update after the campaign is marked completed.
	err := r.q.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM campaign_recipients
		WHERE campaign_id = $1
		  AND status NOT IN ('sent', 'delivered', 'read', 'failed', 'cancelled', 'skipped')
	`, campaignID).Scan(&count)
	if err != nil {
		return 0, repository.MapError(err)
	}
	return count, nil
}

func (r *PostgresRepository) CompleteCampaign(ctx context.Context, teamID, campaignID uuid.UUID, completedAt time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE campaigns SET status = 'completed', completed_at = $1, updated_at = NOW()
		WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
		  AND status IN ('sending', 'queued', 'scheduled')
	`, completedAt, campaignID, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AggregateRecipientStatusCounts(ctx context.Context, campaignID uuid.UUID) (RecipientStatusCounts, error) {
	var counts RecipientStatusCounts
	err := r.q.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('sent', 'delivered', 'read'))::int,
			COUNT(*) FILTER (WHERE status IN ('delivered', 'read'))::int,
			COUNT(*) FILTER (WHERE status = 'read')::int,
			COUNT(*) FILTER (WHERE status = 'failed')::int
		FROM campaign_recipients WHERE campaign_id = $1
	`, campaignID).Scan(&counts.Sent, &counts.Delivered, &counts.Read, &counts.Failed)
	if err != nil {
		return RecipientStatusCounts{}, repository.MapError(err)
	}
	return counts, nil
}

func (r *PostgresRepository) SetCampaignCounters(ctx context.Context, campaignID uuid.UUID, counts RecipientStatusCounts) error {
	_, err := r.q.Exec(ctx, `
		UPDATE campaigns SET
			sent_count = $1,
			delivered_count = $2,
			read_count = $3,
			failed_count = $4,
			updated_at = NOW()
		WHERE id = $5
	`, counts.Sent, counts.Delivered, counts.Read, counts.Failed, campaignID)
	return repository.MapError(err)
}

func (r *PostgresRepository) RepairCampaignCounters(ctx context.Context, campaignID uuid.UUID) (bool, error) {
	var repaired bool
	err := r.q.QueryRow(ctx, `
		WITH agg AS (
			SELECT
				COUNT(*) FILTER (WHERE status IN ('sent', 'delivered', 'read'))::int AS sent,
				COUNT(*) FILTER (WHERE status IN ('delivered', 'read'))::int AS delivered,
				COUNT(*) FILTER (WHERE status = 'read')::int AS read,
				COUNT(*) FILTER (WHERE status = 'failed')::int AS failed
			FROM campaign_recipients WHERE campaign_id = $1
		)
		UPDATE campaigns c SET
			sent_count = agg.sent,
			delivered_count = agg.delivered,
			read_count = agg.read,
			failed_count = agg.failed,
			updated_at = NOW()
		FROM agg
		WHERE c.id = $1
		  AND (
			c.sent_count IS DISTINCT FROM agg.sent OR
			c.delivered_count IS DISTINCT FROM agg.delivered OR
			c.read_count IS DISTINCT FROM agg.read OR
			c.failed_count IS DISTINCT FROM agg.failed
		  )
		RETURNING true
	`, campaignID).Scan(&repaired)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, repository.MapError(err)
	}
	return repaired, nil
}

func (r *PostgresRepository) ListCampaignIDsForReconciliation(ctx context.Context, updatedSince time.Time) ([]uuid.UUID, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id FROM campaigns
		WHERE deleted_at IS NULL
		  AND status IN ('sending', 'completed', 'failed')
		  AND updated_at >= $1
	`, updatedSince)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, repository.MapError(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresRepository) ApplyCounterDelta(ctx context.Context, campaignID uuid.UUID, delta CounterDelta) error {
	_, err := r.q.Exec(ctx, `
		UPDATE campaigns SET
			sent_count = sent_count + $1,
			delivered_count = delivered_count + $2,
			read_count = read_count + $3,
			failed_count = failed_count + $4,
			updated_at = NOW()
		WHERE id = $5
	`, delta.Sent, delta.Delivered, delta.Read, delta.Failed, campaignID)
	return repository.MapError(err)
}

func scanCampaign(row pgx.Row) (*Campaign, error) { return scanCampaignRow(row) }

func scanCampaignRow(row pgx.Row) (*Campaign, error) {
	var c Campaign
	var status string
	var sendMode *string
	err := row.Scan(
		&c.ID, &c.TeamID, &c.ChannelID, &c.TemplateID, &c.SegmentID, &c.Name, &c.Description, &status,
		&c.VariableMapping, &c.SampleValues, &c.Timezone, &sendMode, &c.ScheduledAt, &c.StartedAt, &c.CompletedAt,
		&c.TotalContacts, &c.SentCount, &c.DeliveredCount, &c.ReadCount, &c.FailedCount, &c.ReplyCount, &c.ClickedCount,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan campaign: %w", err)
	}
	c.Status = Status(status)
	if sendMode != nil {
		sm := SendMode(*sendMode)
		c.SendMode = &sm
	}
	return &c, nil
}

func scanJob(row pgx.Row) (*BroadcastJob, error) {
	var j BroadcastJob
	var status string
	err := row.Scan(
		&j.ID, &j.CampaignID, &j.TeamID, &j.ChannelID, &status, &j.TotalRecipients, &j.BatchesTotal,
		&j.BatchesCompleted, &j.RecipientsSent, &j.StartedAt, &j.CompletedAt, &j.CancelledAt,
		&j.CancelReason, &j.ErrorSummary, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.Status = BroadcastJobStatus(status)
	return &j, nil
}

func scanBatchRow(row pgx.Row) (*BroadcastBatch, error) {
	var b BroadcastBatch
	var status string
	err := row.Scan(
		&b.ID, &b.JobID, &b.CampaignID, &b.BatchNumber, &status, &b.RecipientCount, &b.RecipientsSent,
		&b.RecipientsFailed, &b.AttemptCount, &b.NextRetryAt, &b.LockedBy, &b.LockedAt, &b.ErrorSummary,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan batch: %w", err)
	}
	b.Status = BroadcastBatchStatus(status)
	return &b, nil
}

func scanRecipient(row pgx.Row) (*Recipient, error) {
	var rec Recipient
	var status string
	err := row.Scan(
		&rec.ID, &rec.CampaignID, &rec.TeamID, &rec.ContactID, &rec.JobID, &rec.BatchID,
		&rec.ProviderProfileID, &rec.PhoneNumber, &rec.WAID, &rec.Variables, &status,
		&rec.AttemptCount, &rec.LastError, &rec.FailureCode, &rec.WAMID, &rec.SentAt, &rec.DeliveredAt,
		&rec.ReadAt, &rec.FailedAt, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan recipient: %w", err)
	}
	rec.Status = RecipientStatus(status)
	if len(rec.Variables) == 0 {
		rec.Variables = json.RawMessage(`{}`)
	}
	return &rec, nil
}
