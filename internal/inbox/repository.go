package inbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

type Repository interface {
	Save(ctx context.Context, msg *Message) error
	ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Message, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Save(ctx context.Context, msg *Message) error {
	query := `
		INSERT INTO inbox_messages (
			id, team_id, device_id, wamid, sender, recipient, content, direction, received_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		msg.ID,
		msg.TeamID,
		msg.DeviceID,
		msg.WAMID,
		msg.Sender,
		msg.Recipient,
		msg.Content,
		msg.Direction,
		msg.ReceivedAt,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert inbox message: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, team_id, device_id, wamid, sender, recipient, content, direction, received_at
		FROM inbox_messages
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY received_at DESC
		LIMIT $2
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("list inbox messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID, &m.TeamID, &m.DeviceID, &m.WAMID, &m.Sender, &m.Recipient,
			&m.Content, &m.Direction, &m.ReceivedAt,
		); err != nil {
			return nil, fmt.Errorf("scan inbox message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
