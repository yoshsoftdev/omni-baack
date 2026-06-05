package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("conversation not found")

type ListFilter struct {
	Status     *Status
	AssignedTo *uuid.UUID
	Search     string
	Limit      int
}

type Repository interface {
	Create(ctx context.Context, conversation *Conversation) error
	FindByID(ctx context.Context, teamID, conversationID uuid.UUID) (*Conversation, error)
	FindByContactDevice(ctx context.Context, teamID, contactID, deviceID uuid.UUID) (*Conversation, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]ListItem, error)
	GetDetail(ctx context.Context, teamID, conversationID uuid.UUID) (*Detail, error)
	Update(ctx context.Context, conversation *Conversation) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, conversation *Conversation) error {
	query := `
		INSERT INTO conversations (
			id, team_id, contact_id, device_id, status, assigned_to,
			last_message_at, last_message_preview, unread_count, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		conversation.ID, conversation.TeamID, conversation.ContactID, conversation.DeviceID,
		string(conversation.Status), conversation.AssignedTo,
		conversation.LastMessageAt, conversation.LastMessagePreview, conversation.UnreadCount,
		conversation.CreatedAt, conversation.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, conversationID uuid.UUID) (*Conversation, error) {
	query := `
		SELECT id, team_id, contact_id, device_id, status, assigned_to,
		       last_message_at, last_message_preview, unread_count, created_at, updated_at, deleted_at
		FROM conversations
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, conversationID, teamID))
}

func (r *PostgresRepository) FindByContactDevice(ctx context.Context, teamID, contactID, deviceID uuid.UUID) (*Conversation, error) {
	query := `
		SELECT id, team_id, contact_id, device_id, status, assigned_to,
		       last_message_at, last_message_preview, unread_count, created_at, updated_at, deleted_at
		FROM conversations
		WHERE team_id = $1 AND contact_id = $2 AND device_id = $3 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, teamID, contactID, deviceID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]ListItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	args := []any{teamID}
	where := "conv.team_id = $1 AND conv.deleted_at IS NULL"
	argPos := 2

	if filter.Status != nil {
		where += fmt.Sprintf(" AND conv.status = $%d", argPos)
		args = append(args, string(*filter.Status))
		argPos++
	}
	if filter.AssignedTo != nil {
		where += fmt.Sprintf(" AND conv.assigned_to = $%d", argPos)
		args = append(args, *filter.AssignedTo)
		argPos++
	}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND (c.name ILIKE $%d OR c.phone ILIKE $%d)", argPos, argPos)
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}

	query := fmt.Sprintf(`
		SELECT conv.id, conv.contact_id, c.name, c.phone, conv.device_id, conv.status,
		       conv.assigned_to, u.name, u.email, conv.last_message_at, conv.last_message_preview,
		       conv.unread_count
		FROM conversations conv
		JOIN contacts c ON c.id = conv.contact_id
		LEFT JOIN users u ON u.id = conv.assigned_to
		WHERE %s
		ORDER BY conv.last_message_at DESC NULLS LAST, conv.updated_at DESC
		LIMIT $%d
	`, where, argPos)
	args = append(args, limit)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var items []ListItem
	for rows.Next() {
		var item ListItem
		var status string
		if err := rows.Scan(
			&item.ID, &item.ContactID, &item.ContactName, &item.ContactPhone, &item.DeviceID, &status,
			&item.AssignedTo, &item.AssigneeName, &item.AssigneeEmail,
			&item.LastMessageAt, &item.LastMessagePreview, &item.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("scan conversation list: %w", err)
		}
		item.Status = Status(status)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetDetail(ctx context.Context, teamID, conversationID uuid.UUID) (*Detail, error) {
	query := `
		SELECT conv.id, conv.contact_id, c.name, c.phone, conv.device_id, conv.status,
		       conv.assigned_to, u.name, u.email, conv.last_message_at, conv.last_message_preview,
		       conv.unread_count, conv.created_at, conv.updated_at
		FROM conversations conv
		JOIN contacts c ON c.id = conv.contact_id
		LEFT JOIN users u ON u.id = conv.assigned_to
		WHERE conv.id = $1 AND conv.team_id = $2 AND conv.deleted_at IS NULL
	`
	var detail Detail
	var status string
	err := r.db.Pool.QueryRow(ctx, query, conversationID, teamID).Scan(
		&detail.ID, &detail.ContactID, &detail.ContactName, &detail.ContactPhone, &detail.DeviceID, &status,
		&detail.AssignedTo, &detail.AssigneeName, &detail.AssigneeEmail,
		&detail.LastMessageAt, &detail.LastMessagePreview, &detail.UnreadCount,
		&detail.CreatedAt, &detail.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get conversation detail: %w", err)
	}
	detail.Status = Status(status)
	return &detail, nil
}

func (r *PostgresRepository) Update(ctx context.Context, conversation *Conversation) error {
	query := `
		UPDATE conversations
		SET status = $1, assigned_to = $2, last_message_at = $3, last_message_preview = $4,
		    unread_count = $5, updated_at = $6
		WHERE id = $7 AND team_id = $8 AND deleted_at IS NULL
	`
	result, err := r.db.Pool.Exec(ctx, query,
		string(conversation.Status), conversation.AssignedTo,
		conversation.LastMessageAt, conversation.LastMessagePreview,
		conversation.UnreadCount, conversation.UpdatedAt,
		conversation.ID, conversation.TeamID,
	)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Conversation, error) {
	var conv Conversation
	var status string
	var deletedAt *time.Time
	err := row.Scan(
		&conv.ID, &conv.TeamID, &conv.ContactID, &conv.DeviceID, &status, &conv.AssignedTo,
		&conv.LastMessageAt, &conv.LastMessagePreview, &conv.UnreadCount,
		&conv.CreatedAt, &conv.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	conv.Status = Status(status)
	conv.DeletedAt = deletedAt
	return &conv, nil
}

func preview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 200 {
		return content
	}
	return content[:200]
}
