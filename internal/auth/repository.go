package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrRefreshNotFound = errors.New("refresh token not found")

type RefreshRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Revoke(ctx context.Context, hash string) error
}

type PostgresRefreshRepository struct {
	db *database.DB
}

func NewRefreshRepository(db *database.DB) *PostgresRefreshRepository {
	return &PostgresRefreshRepository{db: db}
}

func (r *PostgresRefreshRepository) Create(ctx context.Context, token *RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, team_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		token.ID,
		token.UserID,
		token.TeamID,
		token.TokenHash,
		token.ExpiresAt,
		token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *PostgresRefreshRepository) FindByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, team_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	row := r.db.Pool.QueryRow(ctx, query, hash)

	var token RefreshToken
	var revokedAt *time.Time
	err := row.Scan(
		&token.ID,
		&token.UserID,
		&token.TeamID,
		&token.TokenHash,
		&token.ExpiresAt,
		&revokedAt,
		&token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefreshNotFound
		}
		return nil, fmt.Errorf("scan refresh token: %w", err)
	}
	token.RevokedAt = revokedAt
	return &token, nil
}

func (r *PostgresRefreshRepository) Revoke(ctx context.Context, hash string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`
	_, err := r.db.Pool.Exec(ctx, query, hash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
