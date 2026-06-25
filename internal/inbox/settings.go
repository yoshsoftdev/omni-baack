package inbox

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

type AssignmentMode string

const (
	AssignmentManual      AssignmentMode = "manual"
	AssignmentRoundRobin  AssignmentMode = "round_robin"
	AssignmentLeastLoaded AssignmentMode = "least_loaded"
)

type TeamSettings struct {
	TeamID             uuid.UUID
	AssignmentMode     AssignmentMode
	ResetUnreadOnReply bool
	RoundRobinCursor   int
	UpdatedAt          time.Time
}

type SettingsRepository interface {
	Get(ctx context.Context, teamID uuid.UUID) (*TeamSettings, error)
	UpdateRoundRobinCursor(ctx context.Context, teamID uuid.UUID, cursor int) error
}

type PostgresSettingsRepository struct {
	db *database.DB
}

func NewSettingsRepository(db *database.DB) *PostgresSettingsRepository {
	return &PostgresSettingsRepository{db: db}
}

func (r *PostgresSettingsRepository) Get(ctx context.Context, teamID uuid.UUID) (*TeamSettings, error) {
	var s TeamSettings
	var mode string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT team_id, assignment_mode, reset_unread_on_reply, round_robin_cursor, updated_at
		FROM team_inbox_settings WHERE team_id = $1
	`, teamID).Scan(&s.TeamID, &mode, &s.ResetUnreadOnReply, &s.RoundRobinCursor, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &TeamSettings{
				TeamID:         teamID,
				AssignmentMode: AssignmentManual,
				UpdatedAt:      time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("get inbox settings: %w", err)
	}
	s.AssignmentMode = AssignmentMode(mode)
	return &s, nil
}

func (r *PostgresSettingsRepository) UpdateRoundRobinCursor(ctx context.Context, teamID uuid.UUID, cursor int) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO team_inbox_settings (team_id, round_robin_cursor, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (team_id) DO UPDATE SET round_robin_cursor = EXCLUDED.round_robin_cursor, updated_at = NOW()
	`, teamID, cursor)
	return repository.MapError(err)
}
