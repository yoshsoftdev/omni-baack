package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// SnapshotRepository persists workspace readiness and onboarding wizard state.
type SnapshotRepository interface {
	Upsert(ctx context.Context, snap *ReadinessSnapshot) error
	FindByTeam(ctx context.Context, teamID uuid.UUID) (*ReadinessSnapshot, error)
}

type PostgresSnapshotRepository struct {
	db *database.DB
}

func NewSnapshotRepository(db *database.DB) SnapshotRepository {
	return &PostgresSnapshotRepository{db: db}
}

func NewPostgresSnapshotRepository(db *database.DB) SnapshotRepository {
	return &PostgresSnapshotRepository{db: db}
}

type ReadinessSnapshot struct {
	TeamID          uuid.UUID
	Score           int
	CompletedCount  int
	TotalCount      int
	Status          string
	Checks          []ReadinessCheck
	Onboarding      *OnboardingState
	ComputedAt      time.Time
}

type OnboardingState struct {
	CurrentStep    int   `json:"current_step"`
	CompletedSteps []int `json:"completed_steps"`
}

func (r *PostgresSnapshotRepository) Upsert(ctx context.Context, snap *ReadinessSnapshot) error {
	payload := snapshotPayload{
		Checks:     snap.Checks,
		Onboarding: snap.Onboarding,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO workspace_readiness_snapshots (
			team_id, score, completed_count, total_count, readiness_status, checks, computed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (team_id) DO UPDATE SET
			score = EXCLUDED.score,
			completed_count = EXCLUDED.completed_count,
			total_count = EXCLUDED.total_count,
			readiness_status = EXCLUDED.readiness_status,
			checks = EXCLUDED.checks,
			computed_at = EXCLUDED.computed_at
	`, snap.TeamID, snap.Score, snap.CompletedCount, snap.TotalCount, snap.Status, raw, snap.ComputedAt)
	return repository.MapError(err)
}

func (r *PostgresSnapshotRepository) FindByTeam(ctx context.Context, teamID uuid.UUID) (*ReadinessSnapshot, error) {
	var snap ReadinessSnapshot
	var raw []byte
	err := r.db.Pool.QueryRow(ctx, `
		SELECT team_id, score, completed_count, total_count, readiness_status, checks, computed_at
		FROM workspace_readiness_snapshots WHERE team_id = $1
	`, teamID).Scan(&snap.TeamID, &snap.Score, &snap.CompletedCount, &snap.TotalCount, &snap.Status, &raw, &snap.ComputedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, repository.MapError(err)
	}
	var payload snapshotPayload
	if err := json.Unmarshal(raw, &payload); err == nil {
		snap.Checks = payload.Checks
		snap.Onboarding = payload.Onboarding
	} else {
		var legacy []ReadinessCheck
		if err := json.Unmarshal(raw, &legacy); err == nil {
			snap.Checks = legacy
		}
	}
	return &snap, nil
}

type snapshotPayload struct {
	Checks     []ReadinessCheck `json:"checks"`
	Onboarding *OnboardingState `json:"onboarding,omitempty"`
}
