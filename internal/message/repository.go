package message

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

type Repository interface {
	Create(ctx context.Context, message *Message) error
	ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, limit int) ([]Message, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, message *Message) error {
	query := `
		INSERT INTO messages (
			id, team_id, conversation_id, device_id, contact_id, wamid,
			direction, content, sender_phone, sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		message.ID, message.TeamID, message.ConversationID, message.DeviceID, message.ContactID,
		message.WAMID, message.Direction, message.Content, message.SenderPhone,
		message.SentAt, message.CreatedAt, message.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT id, team_id, conversation_id, device_id, contact_id, wamid,
		       direction, content, sender_phone, sent_at, created_at, updated_at, deleted_at
		FROM messages
		WHERE team_id = $1 AND conversation_id = $2 AND deleted_at IS NULL
		ORDER BY sent_at ASC
		LIMIT $3
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var deletedAt *time.Time
		if err := rows.Scan(
			&m.ID, &m.TeamID, &m.ConversationID, &m.DeviceID, &m.ContactID, &m.WAMID,
			&m.Direction, &m.Content, &m.SenderPhone, &m.SentAt, &m.CreatedAt, &m.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.DeletedAt = deletedAt
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *PostgresRepository) FindByWAMID(ctx context.Context, wamid string) (*Message, error) {
	query := `
		SELECT id, team_id, conversation_id, device_id, contact_id, wamid,
		       direction, content, sender_phone, sent_at, created_at, updated_at, deleted_at
		FROM messages WHERE wamid = $1 AND deleted_at IS NULL
	`
	var m Message
	var deletedAt *time.Time
	err := r.db.Pool.QueryRow(ctx, query, wamid).Scan(
		&m.ID, &m.TeamID, &m.ConversationID, &m.DeviceID, &m.ContactID, &m.WAMID,
		&m.Direction, &m.Content, &m.SenderPhone, &m.SentAt, &m.CreatedAt, &m.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find message by wamid: %w", err)
	}
	m.DeletedAt = deletedAt
	return &m, nil
}
