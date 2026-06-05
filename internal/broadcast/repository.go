package broadcast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("broadcast not found")

type Repository interface {
	Create(ctx context.Context, broadcast *Broadcast) error
	FindByID(ctx context.Context, teamID, broadcastID uuid.UUID) (*Broadcast, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Broadcast, error)
	Update(ctx context.Context, broadcast *Broadcast) error
	SoftDelete(ctx context.Context, teamID, broadcastID uuid.UUID) error
	AddRecipients(ctx context.Context, recipients []Recipient) error
	ListRecipients(ctx context.Context, broadcastID uuid.UUID) ([]Recipient, error)
	ListPendingRecipients(ctx context.Context, broadcastID uuid.UUID) ([]Recipient, error)
	UpdateRecipient(ctx context.Context, recipient *Recipient) error
	AddBroadcastTags(ctx context.Context, broadcastID uuid.UUID, tagIDs []uuid.UUID) error
	ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Broadcast, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, broadcast *Broadcast) error {
	query := `
		INSERT INTO broadcasts (
			id, team_id, device_id, name, message, status, scheduled_at,
			started_at, completed_at, created_by, total_recipients, sent_count, failed_count,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		broadcast.ID, broadcast.TeamID, broadcast.DeviceID, broadcast.Name, broadcast.Message,
		string(broadcast.Status), broadcast.ScheduledAt, broadcast.StartedAt, broadcast.CompletedAt,
		broadcast.CreatedBy, broadcast.TotalRecipients, broadcast.SentCount, broadcast.FailedCount,
		broadcast.CreatedAt, broadcast.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert broadcast: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, broadcastID uuid.UUID) (*Broadcast, error) {
	query := `
		SELECT id, team_id, device_id, name, message, status, scheduled_at, started_at, completed_at,
		       created_by, total_recipients, sent_count, failed_count, created_at, updated_at, deleted_at
		FROM broadcasts
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, broadcastID, teamID))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Broadcast, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, team_id, device_id, name, message, status, scheduled_at, started_at, completed_at,
		       created_by, total_recipients, sent_count, failed_count, created_at, updated_at, deleted_at
		FROM broadcasts
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("list broadcasts: %w", err)
	}
	defer rows.Close()

	var items []Broadcast
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, broadcast *Broadcast) error {
	query := `
		UPDATE broadcasts
		SET name = $1, message = $2, status = $3, scheduled_at = $4, started_at = $5,
		    completed_at = $6, total_recipients = $7, sent_count = $8, failed_count = $9, updated_at = $10
		WHERE id = $11 AND team_id = $12 AND deleted_at IS NULL
	`
	result, err := r.db.Pool.Exec(ctx, query,
		broadcast.Name, broadcast.Message, string(broadcast.Status), broadcast.ScheduledAt,
		broadcast.StartedAt, broadcast.CompletedAt, broadcast.TotalRecipients, broadcast.SentCount,
		broadcast.FailedCount, broadcast.UpdatedAt, broadcast.ID, broadcast.TeamID,
	)
	if err != nil {
		return fmt.Errorf("update broadcast: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, broadcastID uuid.UUID) error {
	result, err := r.db.Pool.Exec(ctx, `
		UPDATE broadcasts SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, broadcastID, teamID)
	if err != nil {
		return fmt.Errorf("delete broadcast: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AddRecipients(ctx context.Context, recipients []Recipient) error {
	if len(recipients) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	query := `
		INSERT INTO broadcast_recipients (id, broadcast_id, contact_id, phone, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	for _, recipient := range recipients {
		batch.Queue(query,
			recipient.ID, recipient.BroadcastID, recipient.ContactID,
			recipient.Phone, recipient.Status, recipient.CreatedAt,
		)
	}
	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range recipients {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert broadcast recipient: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) ListRecipients(ctx context.Context, broadcastID uuid.UUID) ([]Recipient, error) {
	query := `
		SELECT id, broadcast_id, contact_id, phone, status, wamid, error_message, sent_at, created_at
		FROM broadcast_recipients
		WHERE broadcast_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("list broadcast recipients: %w", err)
	}
	defer rows.Close()
	return r.scanRecipients(rows)
}

func (r *PostgresRepository) ListPendingRecipients(ctx context.Context, broadcastID uuid.UUID) ([]Recipient, error) {
	query := `
		SELECT id, broadcast_id, contact_id, phone, status, wamid, error_message, sent_at, created_at
		FROM broadcast_recipients
		WHERE broadcast_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("list pending recipients: %w", err)
	}
	defer rows.Close()
	return r.scanRecipients(rows)
}

func (r *PostgresRepository) UpdateRecipient(ctx context.Context, recipient *Recipient) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE broadcast_recipients
		SET status = $1, wamid = $2, error_message = $3, sent_at = $4
		WHERE id = $5
	`, recipient.Status, recipient.WAMID, recipient.ErrorMessage, recipient.SentAt, recipient.ID)
	if err != nil {
		return fmt.Errorf("update broadcast recipient: %w", err)
	}
	return nil
}

func (r *PostgresRepository) AddBroadcastTags(ctx context.Context, broadcastID uuid.UUID, tagIDs []uuid.UUID) error {
	if len(tagIDs) == 0 {
		return nil
	}
	for _, tagID := range tagIDs {
		_, err := r.db.Pool.Exec(ctx, `
			INSERT INTO broadcast_tags (broadcast_id, tag_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, broadcastID, tagID)
		if err != nil {
			return fmt.Errorf("insert broadcast tag: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Broadcast, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, team_id, device_id, name, message, status, scheduled_at, started_at, completed_at,
		       created_by, total_recipients, sent_count, failed_count, created_at, updated_at, deleted_at
		FROM broadcasts
		WHERE deleted_at IS NULL AND status = 'scheduled' AND scheduled_at <= $1
		ORDER BY scheduled_at ASC
		LIMIT $2
	`
	rows, err := r.db.Pool.Query(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due broadcasts: %w", err)
	}
	defer rows.Close()

	var items []Broadcast
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Broadcast, error) {
	var b Broadcast
	var status string
	var deletedAt *time.Time
	err := row.Scan(
		&b.ID, &b.TeamID, &b.DeviceID, &b.Name, &b.Message, &status,
		&b.ScheduledAt, &b.StartedAt, &b.CompletedAt, &b.CreatedBy,
		&b.TotalRecipients, &b.SentCount, &b.FailedCount,
		&b.CreatedAt, &b.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan broadcast: %w", err)
	}
	b.Status = Status(status)
	b.DeletedAt = deletedAt
	return &b, nil
}

func (r *PostgresRepository) scanRow(rows pgx.Rows) (*Broadcast, error) {
	var b Broadcast
	var status string
	var deletedAt *time.Time
	err := rows.Scan(
		&b.ID, &b.TeamID, &b.DeviceID, &b.Name, &b.Message, &status,
		&b.ScheduledAt, &b.StartedAt, &b.CompletedAt, &b.CreatedBy,
		&b.TotalRecipients, &b.SentCount, &b.FailedCount,
		&b.CreatedAt, &b.UpdatedAt, &deletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan broadcast row: %w", err)
	}
	b.Status = Status(status)
	b.DeletedAt = deletedAt
	return &b, nil
}

func (r *PostgresRepository) scanRecipients(rows pgx.Rows) ([]Recipient, error) {
	var recipients []Recipient
	for rows.Next() {
		var rcp Recipient
		if err := rows.Scan(
			&rcp.ID, &rcp.BroadcastID, &rcp.ContactID, &rcp.Phone, &rcp.Status,
			&rcp.WAMID, &rcp.ErrorMessage, &rcp.SentAt, &rcp.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan broadcast recipient: %w", err)
		}
		recipients = append(recipients, rcp)
	}
	return recipients, rows.Err()
}
