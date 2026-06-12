package otp

import (
	"context"
	"fmt"

	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

type Repository interface {
	Create(ctx context.Context, log *Log) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, log *Log) error {
	const query = `
		INSERT INTO otp_logs (
			id, team_id, device_id, phone, wamid, status, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(
		ctx,
		query,
		log.ID,
		log.TeamID,
		log.DeviceID,
		log.Phone,
		log.Wamid,
		string(log.Status),
		log.ErrorMessage,
		log.CreatedAt,
		log.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert otp log: %w", err)
	}
	return nil
}
