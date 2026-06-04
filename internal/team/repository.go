package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var (
	ErrNotFound       = errors.New("team not found")
	ErrMemberNotFound = errors.New("team member not found")
	ErrAlreadyMember  = errors.New("user is already a team member")
)

type Repository interface {
	Create(ctx context.Context, team *Team) error
	FindByID(ctx context.Context, id uuid.UUID) (*Team, error)
	Update(ctx context.Context, team *Team) error
	CreateMember(ctx context.Context, member *Member) error
	FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*Member, error)
	FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*Member, error)
	FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*Member, error)
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]MemberWithUser, error)
	UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role Role) error
	SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, team *Team) error {
	query := `
		INSERT INTO teams (id, name, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		team.ID,
		team.Name,
		team.Slug,
		team.CreatedAt,
		team.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("team slug already exists")
		}
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	query := `
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM teams
		WHERE id = $1 AND deleted_at IS NULL
	`
	return r.scanTeam(r.db.Pool.QueryRow(ctx, query, id))
}

func (r *PostgresRepository) Update(ctx context.Context, team *Team) error {
	query := `
		UPDATE teams
		SET name = $1, slug = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, team.Name, team.Slug, team.UpdatedAt, team.ID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("team slug already exists")
		}
		return fmt.Errorf("update team: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateMember(ctx context.Context, member *Member) error {
	query := `
		INSERT INTO team_members (id, team_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		member.ID,
		member.TeamID,
		member.UserID,
		string(member.Role),
		member.CreatedAt,
		member.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyMember
		}
		return fmt.Errorf("insert team member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
		FROM team_members
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1
	`
	return r.scanMember(r.db.Pool.QueryRow(ctx, query, userID))
}

func (r *PostgresRepository) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`
	return r.scanMember(r.db.Pool.QueryRow(ctx, query, teamID, userID))
}

func (r *PostgresRepository) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*Member, error) {
	query := `
		SELECT id, team_id, user_id, role, created_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
	`
	return r.scanMember(r.db.Pool.QueryRow(ctx, query, teamID, memberID))
}

func (r *PostgresRepository) ListMembers(ctx context.Context, teamID uuid.UUID) ([]MemberWithUser, error) {
	query := `
		SELECT tm.id, tm.user_id, tm.role, tm.created_at, u.email, u.name
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1 AND tm.deleted_at IS NULL AND u.deleted_at IS NULL
		ORDER BY tm.created_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	var members []MemberWithUser
	for rows.Next() {
		var m MemberWithUser
		var role string
		if err := rows.Scan(&m.ID, &m.UserID, &role, &m.CreatedAt, &m.Email, &m.Name); err != nil {
			return nil, fmt.Errorf("scan team member row: %w", err)
		}
		m.Role = Role(role)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team members: %w", err)
	}
	return members, nil
}

func (r *PostgresRepository) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role Role) error {
	query := `
		UPDATE team_members
		SET role = $1, updated_at = NOW()
		WHERE team_id = $2 AND id = $3 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, string(role), teamID, memberID)
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error {
	query := `
		UPDATE team_members
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, teamID, memberID)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *PostgresRepository) scanTeam(row pgx.Row) (*Team, error) {
	var team Team
	var deletedAt *time.Time
	err := row.Scan(
		&team.ID,
		&team.Name,
		&team.Slug,
		&team.CreatedAt,
		&team.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan team: %w", err)
	}
	team.DeletedAt = deletedAt
	return &team, nil
}

func (r *PostgresRepository) scanMember(row pgx.Row) (*Member, error) {
	var member Member
	var role string
	var deletedAt *time.Time
	err := row.Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&role,
		&member.CreatedAt,
		&member.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMemberNotFound
		}
		return nil, fmt.Errorf("scan team member: %w", err)
	}
	member.Role = Role(role)
	member.DeletedAt = deletedAt
	return &member, nil
}
