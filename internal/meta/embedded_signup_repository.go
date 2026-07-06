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
			id, team_id, user_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		session.ID, session.TeamID, session.UserID, session.ChannelID, session.BusinessID, string(session.Status),
		session.OAuthState, session.OAuthCodeHash, session.ErrorMessage,
		session.CreatedAt, session.ExpiresAt, session.CompletedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, user_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE id = $1 AND team_id = $2
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByOAuthState(ctx context.Context, teamID uuid.UUID, oauthState string) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, user_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE team_id = $1 AND oauth_state = $2
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, teamID, oauthState))
}

func (r *PostgresEmbeddedSignupSessionRepository) FindByOAuthCodeHash(ctx context.Context, teamID uuid.UUID, codeHash string) (*EmbeddedSignupSession, error) {
	query := `
		SELECT id, team_id, user_id, channel_id, business_id, status, oauth_state, oauth_code_hash,
			error_message, created_at, expires_at, completed_at
		FROM embedded_signup_sessions
		WHERE team_id = $1 AND oauth_code_hash = $2
	`
	return scanEmbeddedSignupSession(r.db.Pool.QueryRow(ctx, query, teamID, codeHash))
}

func (r *PostgresEmbeddedSignupSessionRepository) Update(ctx context.Context, session *EmbeddedSignupSession) error {
	query := `
		UPDATE embedded_signup_sessions SET
			user_id = $1, channel_id = $2, business_id = $3, status = $4, oauth_code_hash = $5, error_message = $6,
			expires_at = $7, completed_at = $8
		WHERE id = $9 AND team_id = $10
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		session.UserID, session.ChannelID, session.BusinessID, string(session.Status), session.OAuthCodeHash, session.ErrorMessage,
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
		&s.ID, &s.TeamID, &s.UserID, &s.ChannelID, &s.BusinessID, &status, &s.OAuthState, &s.OAuthCodeHash,
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
