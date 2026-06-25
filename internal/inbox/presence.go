package inbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type PresenceStatus string

const (
	PresenceOnline  PresenceStatus = "online"
	PresenceOffline PresenceStatus = "offline"
)

type MemberPresence struct {
	TeamID     uuid.UUID
	UserID     uuid.UUID
	Status     PresenceStatus
	LastSeenAt time.Time
}

type PresenceRepository interface {
	Upsert(ctx context.Context, p *MemberPresence) error
	ListByTeam(ctx context.Context, teamID uuid.UUID) ([]MemberPresence, error)
}

type PostgresPresenceRepository struct {
	db *database.DB
}

func NewPresenceRepository(db *database.DB) *PostgresPresenceRepository {
	return &PostgresPresenceRepository{db: db}
}

func (r *PostgresPresenceRepository) Upsert(ctx context.Context, p *MemberPresence) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO team_member_presence (team_id, user_id, status, last_seen_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (team_id, user_id) DO UPDATE SET
			status = EXCLUDED.status,
			last_seen_at = EXCLUDED.last_seen_at
	`, p.TeamID, p.UserID, string(p.Status), p.LastSeenAt)
	return repository.MapError(err)
}

func (r *PostgresPresenceRepository) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]MemberPresence, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT team_id, user_id, status, last_seen_at
		FROM team_member_presence WHERE team_id = $1
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list presence: %w", err)
	}
	defer rows.Close()
	out := make([]MemberPresence, 0)
	for rows.Next() {
		var p MemberPresence
		var status string
		if err := rows.Scan(&p.TeamID, &p.UserID, &status, &p.LastSeenAt); err != nil {
			return nil, err
		}
		p.Status = PresenceStatus(status)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PresenceService tracks team member online state.
type PresenceService struct {
	repo PresenceRepository
}

func NewPresenceService(repo PresenceRepository) *PresenceService {
	return &PresenceService{repo: repo}
}

func (s *PresenceService) SetOnline(ctx context.Context, teamID, userID uuid.UUID) error {
	return s.repo.Upsert(ctx, &MemberPresence{
		TeamID: teamID, UserID: userID, Status: PresenceOnline, LastSeenAt: time.Now().UTC(),
	})
}

func (s *PresenceService) SetOffline(ctx context.Context, teamID, userID uuid.UUID) error {
	return s.repo.Upsert(ctx, &MemberPresence{
		TeamID: teamID, UserID: userID, Status: PresenceOffline, LastSeenAt: time.Now().UTC(),
	})
}

func (s *PresenceService) List(ctx context.Context, teamID uuid.UUID) ([]MemberPresence, error) {
	return s.repo.ListByTeam(ctx, teamID)
}

// scan guard for pgx
var _ = pgx.ErrNoRows
