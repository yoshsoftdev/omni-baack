package tag

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

var ErrNotFound = errors.New("tag not found")

type Repository interface {
	Create(ctx context.Context, tag *Tag) error
	FindByID(ctx context.Context, teamID, tagID uuid.UUID) (*Tag, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Tag, error)
	Update(ctx context.Context, tag *Tag) error
	SoftDelete(ctx context.Context, teamID, tagID uuid.UUID) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, tag *Tag) error {
	query := `
		INSERT INTO tags (id, team_id, name, color, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		tag.ID, tag.TeamID, tag.Name, tag.Color, tag.CreatedAt, tag.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("tag name already exists")
		}
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, tagID uuid.UUID) (*Tag, error) {
	query := `
		SELECT id, team_id, name, color, created_at, updated_at, deleted_at
		FROM tags
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, tagID, teamID))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Tag, error) {
	query := `
		SELECT id, team_id, name, color, created_at, updated_at, deleted_at
		FROM tags
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *t)
	}
	return tags, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, tag *Tag) error {
	query := `
		UPDATE tags
		SET name = $1, color = $2, updated_at = $3
		WHERE id = $4 AND team_id = $5 AND deleted_at IS NULL
	`
	result, err := r.db.Pool.Exec(ctx, query, tag.Name, tag.Color, tag.UpdatedAt, tag.ID, tag.TeamID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("tag name already exists")
		}
		return fmt.Errorf("update tag: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, tagID uuid.UUID) error {
	query := `
		UPDATE tags SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, tagID, teamID)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Tag, error) {
	var t Tag
	var deletedAt *time.Time
	err := row.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt, &deletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan tag: %w", err)
	}
	t.DeletedAt = deletedAt
	return &t, nil
}

func (r *PostgresRepository) scanRow(rows pgx.Rows) (*Tag, error) {
	var t Tag
	var deletedAt *time.Time
	err := rows.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt, &deletedAt)
	if err != nil {
		return nil, fmt.Errorf("scan tag row: %w", err)
	}
	t.DeletedAt = deletedAt
	return &t, nil
}
