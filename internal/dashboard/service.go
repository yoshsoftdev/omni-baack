package dashboard

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/workspace"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type Overview struct {
	Contacts                int64    `json:"contacts"`
	Channels                int64    `json:"channels"`
	Campaigns               int64    `json:"campaigns"`
	UnreadConversations     int64    `json:"unread_conversations"`
	OpenConversations       int64    `json:"open_conversations"`
	UnassignedConversations int64    `json:"unassigned_conversations"`
	AvgFirstResponseSeconds *float64 `json:"avg_first_response_seconds,omitempty"`
	AvgResolutionSeconds    *float64 `json:"avg_resolution_seconds,omitempty"`
	ReadinessScore          int      `json:"readiness_score"`
}

type Repository interface {
	CountContacts(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountChannels(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountCampaigns(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountUnreadConversations(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountOpenConversations(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountUnassignedConversations(ctx context.Context, teamID uuid.UUID) (int64, error)
	AvgFirstResponseSeconds(ctx context.Context, teamID uuid.UUID) (*float64, error)
	AvgResolutionSeconds(ctx context.Context, teamID uuid.UUID) (*float64, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) Repository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CountContacts(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM contacts WHERE team_id = $1 AND deleted_at IS NULL`, teamID)
}

func (r *PostgresRepository) CountChannels(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM channels WHERE team_id = $1 AND deleted_at IS NULL`, teamID)
}

func (r *PostgresRepository) CountCampaigns(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM campaigns WHERE team_id = $1 AND deleted_at IS NULL`, teamID)
}

func (r *PostgresRepository) CountUnreadConversations(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM conversations WHERE team_id = $1 AND deleted_at IS NULL AND unread_count > 0`, teamID)
}

func (r *PostgresRepository) CountOpenConversations(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM conversations WHERE team_id = $1 AND deleted_at IS NULL AND status IN ('open','pending','assigned')`, teamID)
}

func (r *PostgresRepository) CountUnassignedConversations(ctx context.Context, teamID uuid.UUID) (int64, error) {
	return countQuery(ctx, r.db, `SELECT COUNT(*) FROM conversations WHERE team_id = $1 AND deleted_at IS NULL AND assigned_to IS NULL`, teamID)
}

func (r *PostgresRepository) AvgFirstResponseSeconds(ctx context.Context, teamID uuid.UUID) (*float64, error) {
	return avgSecondsQuery(ctx, r.db, `
		SELECT AVG(EXTRACT(EPOCH FROM (first_response_at - last_customer_message_at)))
		FROM conversations
		WHERE team_id = $1 AND deleted_at IS NULL
			AND first_response_at IS NOT NULL AND last_customer_message_at IS NOT NULL
	`, teamID)
}

func (r *PostgresRepository) AvgResolutionSeconds(ctx context.Context, teamID uuid.UUID) (*float64, error) {
	return avgSecondsQuery(ctx, r.db, `
		SELECT AVG(EXTRACT(EPOCH FROM (resolution_at - created_at)))
		FROM conversations
		WHERE team_id = $1 AND deleted_at IS NULL AND resolution_at IS NOT NULL
	`, teamID)
}

func avgSecondsQuery(ctx context.Context, db *database.DB, query string, teamID uuid.UUID) (*float64, error) {
	var avg *float64
	err := db.Pool.QueryRow(ctx, query, teamID).Scan(&avg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return avg, nil
}

func countQuery(ctx context.Context, db *database.DB, query string, teamID uuid.UUID) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, query, teamID).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

type Service struct {
	repo      Repository
	teams     team.Repository
	workspace *workspace.Service
	rbac      *rbac.Service
}

func NewService(repo Repository, teams team.Repository, workspaceSvc *workspace.Service, rbacSvc *rbac.Service) *Service {
	return &Service{repo: repo, teams: teams, workspace: workspaceSvc, rbac: rbacSvc}
}

func (s *Service) GetOverview(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*Overview, error) {
	if s.rbac != nil && !s.rbac.Require(actorRole, rbac.PermUserView) {
		return nil, serviceerrors.ErrForbidden
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		if errors.Is(err, team.ErrMemberNotFound) {
			return nil, serviceerrors.ErrForbidden
		}
		return nil, err
	}

	contacts, err := s.repo.CountContacts(ctx, teamID)
	if err != nil {
		return nil, err
	}
	channels, err := s.repo.CountChannels(ctx, teamID)
	if err != nil {
		return nil, err
	}
	campaigns, err := s.repo.CountCampaigns(ctx, teamID)
	if err != nil {
		return nil, err
	}
	unread, err := s.repo.CountUnreadConversations(ctx, teamID)
	if err != nil {
		return nil, err
	}
	open, err := s.repo.CountOpenConversations(ctx, teamID)
	if err != nil {
		return nil, err
	}
	unassigned, err := s.repo.CountUnassignedConversations(ctx, teamID)
	if err != nil {
		return nil, err
	}
	avgFirst, err := s.repo.AvgFirstResponseSeconds(ctx, teamID)
	if err != nil {
		return nil, err
	}
	avgResolution, err := s.repo.AvgResolutionSeconds(ctx, teamID)
	if err != nil {
		return nil, err
	}

	readinessScore := 0
	if s.workspace != nil {
		readinessScore = s.workspace.ReadinessScoreFromSnapshot(ctx, teamID)
	}

	return &Overview{
		Contacts:                contacts,
		Channels:                channels,
		Campaigns:               campaigns,
		UnreadConversations:     unread,
		OpenConversations:       open,
		UnassignedConversations: unassigned,
		AvgFirstResponseSeconds: avgFirst,
		AvgResolutionSeconds:    avgResolution,
		ReadinessScore:          readinessScore,
	}, nil
}
