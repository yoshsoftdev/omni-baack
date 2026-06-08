package template

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("template not found")

type Repository interface {
	Upsert(ctx context.Context, tpl *Template) error
	FindByID(ctx context.Context, teamID, templateID uuid.UUID) (*Template, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]Template, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Upsert(ctx context.Context, tpl *Template) error {
	query := `
		INSERT INTO templates (
			id, team_id, device_id, waba_id, meta_template_id, name, language,
			category, status, components, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (team_id, meta_template_id, language) WHERE deleted_at IS NULL
		DO UPDATE SET
			device_id = EXCLUDED.device_id,
			waba_id = EXCLUDED.waba_id,
			name = EXCLUDED.name,
			category = EXCLUDED.category,
			status = EXCLUDED.status,
			components = EXCLUDED.components,
			updated_at = EXCLUDED.updated_at
		RETURNING id
	`
	var id uuid.UUID
	err := r.db.Pool.QueryRow(ctx, query,
		tpl.ID, tpl.TeamID, tpl.DeviceID, tpl.WabaID, tpl.MetaTemplateID,
		tpl.Name, tpl.Language, tpl.Category, tpl.Status, tpl.Components,
		tpl.CreatedAt, tpl.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	tpl.ID = id
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, templateID uuid.UUID) (*Template, error) {
	query := `
		SELECT id, team_id, device_id, waba_id, meta_template_id, name, language,
			category, status, components, created_at, updated_at, deleted_at
		FROM templates
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, templateID, teamID))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]Template, error) {
	query := `
		SELECT id, team_id, device_id, waba_id, meta_template_id, name, language,
			category, status, components, created_at, updated_at, deleted_at
		FROM templates
		WHERE team_id = $1 AND deleted_at IS NULL
	`
	args := []any{teamID}
	argPos := 2
	if filter.DeviceID != nil && *filter.DeviceID != "" {
		query += fmt.Sprintf(` AND device_id = $%d`, argPos)
		args = append(args, *filter.DeviceID)
		argPos++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(` AND (name ILIKE $%d OR language ILIKE $%d)`, argPos, argPos)
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}
	if filter.Language != "" && filter.Language != "all" {
		query += fmt.Sprintf(` AND language ILIKE $%d`, argPos)
		args = append(args, "%"+filter.Language+"%")
		argPos++
	}
	if filter.Category != "" && filter.Category != "all" {
		query += fmt.Sprintf(` AND category ILIKE $%d`, argPos)
		args = append(args, filter.Category)
		argPos++
	}
	if filter.Status != "" && filter.Status != "all" {
		query += fmt.Sprintf(` AND status ILIKE $%d`, argPos)
		args = append(args, filter.Status)
		argPos++
	}
	_ = argPos
	query += ` ORDER BY name ASC, language ASC`

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var items []Template
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Template, error) {
	var tpl Template
	var components []byte
	err := row.Scan(
		&tpl.ID, &tpl.TeamID, &tpl.DeviceID, &tpl.WabaID, &tpl.MetaTemplateID,
		&tpl.Name, &tpl.Language, &tpl.Category, &tpl.Status, &components,
		&tpl.CreatedAt, &tpl.UpdatedAt, &tpl.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan template: %w", err)
	}
	tpl.Components = json.RawMessage(components)
	return &tpl, nil
}

func (r *PostgresRepository) scanRow(rows pgx.Rows) (*Template, error) {
	var tpl Template
	var components []byte
	err := rows.Scan(
		&tpl.ID, &tpl.TeamID, &tpl.DeviceID, &tpl.WabaID, &tpl.MetaTemplateID,
		&tpl.Name, &tpl.Language, &tpl.Category, &tpl.Status, &components,
		&tpl.CreatedAt, &tpl.UpdatedAt, &tpl.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan template row: %w", err)
	}
	tpl.Components = json.RawMessage(components)
	return &tpl, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
