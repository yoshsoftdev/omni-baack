package meta

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type PostgresEmbeddedSignupSessionRepository struct {
	db *database.DB
}

func NewEmbeddedSignupSessionRepository(db *database.DB) *PostgresEmbeddedSignupSessionRepository {
	return &PostgresEmbeddedSignupSessionRepository{db: db}
}

func (r *PostgresEmbeddedSignupSessionRepository) Create(ctx context.Context, session *EmbeddedSignupSession) error {
	query := `
		INSERT INTO embedded_signup_sessions (
			id, team_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		session.ID, session.TeamID, session.ChannelID, session.BusinessID, string(session.Status),
		session.OAuthState, session.OAuthCodeHash, session.ErrorMessage,
		session.CreatedAt, session.ExpiresAt, session.CompletedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE id = $1 AND team_id = $2
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByOAuthState(ctx context.Context, oauthState string) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE oauth_state = $1
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, oauthState))
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByOAuthCodeHash(ctx context.Context, codeHash string) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE oauth_code_hash = $1
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, codeHash))
}

func (r *PostgresEmbeddedSignupSessionRepository) Update(ctx context.Context, session *EmbeddedSignupSession) error {
	query := `
		UPDATE embedded_signup_sessions SET
			business_id = $1, status = $2, oauth_code_hash = $3, error_message = $4,
			expires_at = $5, completed_at = $6
		WHERE id = $7 AND team_id = $8
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		session.BusinessID, string(session.Status), session.OAuthCodeHash, session.ErrorMessage,
		session.ExpiresAt, session.CompletedAt, session.ID, session.TeamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanEmbeddedSignupSession(row pgx.Row) (*EmbeddedSignupSession, error) {
	var s EmbeddedSignupSession
	var status string
	err := row.Scan(
		&s.ID, &s.TeamID, &s.ChannelID, &s.BusinessID, &status, &s.OAuthState, &s.OAuthCodeHash,
		&s.ErrorMessage, &s.CreatedAt, &s.ExpiresAt, &s.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan embedded signup session: %w", err)
	}
	s.Status = EmbeddedSignupSessionStatus(status)
	return &s, nil
}
