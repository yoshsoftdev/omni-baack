package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (UserRepository) persists users and refresh tokens.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, u *User) error
	UpdateLastActive(ctx context.Context, id uuid.UUID, at time.Time) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	CreateRefreshToken(ctx context.Context, t *RefreshToken) error
	FindRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID, at time.Time) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, u *User) error {
	query := `
		INSERT INTO users (id, email, name, password_hash, last_active_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		u.ID, u.Email, u.Name, u.PasswordHash, u.LastActiveAt, u.CreatedAt, u.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, email, name, password_hash, last_active_at, created_at, updated_at, deleted_at
		FROM users WHERE id = $1 AND deleted_at IS NULL
	`
	return scanUser(r.db.Pool.QueryRow(ctx, query, id))
}

func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, name, password_hash, last_active_at, created_at, updated_at, deleted_at
		FROM users WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL
	`
	return scanUser(r.db.Pool.QueryRow(ctx, query, email))
}

func (r *PostgresRepository) Update(ctx context.Context, u *User) error {
	query := `
		UPDATE users SET email = $1, name = $2, password_hash = $3, updated_at = $4
		WHERE id = $5 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, u.Email, u.Name, u.PasswordHash, u.UpdatedAt, u.ID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateLastActive(ctx context.Context, id uuid.UUID, at time.Time) error {
	query := `UPDATE users SET last_active_at = $1, updated_at = $1 WHERE id = $2 AND deleted_at IS NULL`
	_, err := r.db.Pool.Exec(ctx, query, at, id)
	return repository.MapError(err)
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	tag, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateRefreshToken(ctx context.Context, t *RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, team_id, token_hash, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		t.ID, t.UserID, t.TeamID, t.TokenHash, t.ExpiresAt, t.RevokedAt, t.CreatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, team_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1 AND revoked_at IS NULL
	`
	var t RefreshToken
	err := r.db.Pool.QueryRow(ctx, query, hash).Scan(
		&t.ID, &t.UserID, &t.TeamID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return &t, nil
}

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID, at time.Time) error {
	query := `UPDATE refresh_tokens SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL`
	tag, err := r.db.Pool.Exec(ctx, query, at, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastActiveAt,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}
