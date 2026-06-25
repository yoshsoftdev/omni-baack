package template

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

// Repository (TemplateRepository) persists Meta templates.
type Repository interface {
	Create(ctx context.Context, t *Template) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Template, error)
	FindByMetaTemplateID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Template], error)
	UpsertFromSync(ctx context.Context, t *Template) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	SoftDeleteExceptMetaIDs(ctx context.Context, teamID, channelID uuid.UUID, keepMetaIDs []string) (int64, error)
	HasRecentSync(ctx context.Context, teamID uuid.UUID, within time.Duration) (bool, error)
	UpdateFromWebhook(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID, name string, status Status, category *string, updatedAt time.Time) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t *Template) error {
	query := `
		INSERT INTO templates (
			id, channel_id, team_id, waba_id, meta_template_id, name, language,
			category, status, preview, components, variable_count, last_sync_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		t.ID, t.ChannelID, t.TeamID, t.WABAID, t.MetaTemplateID, t.Name, t.Language,
		t.Category, string(t.Status), t.Preview, t.Components, t.VariableCount, t.LastSyncAt,
		t.CreatedAt, t.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Template, error) {
	query := `
		SELECT id, channel_id, team_id, waba_id, meta_template_id, name, language,
			category, status, preview, components, variable_count, last_sync_at, created_at, updated_at, deleted_at
		FROM templates WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return scanTemplate(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresRepository) FindByMetaTemplateID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error) {
	query := `
		SELECT id, channel_id, team_id, waba_id, meta_template_id, name, language,
			category, status, preview, components, variable_count, last_sync_at, created_at, updated_at, deleted_at
		FROM templates
		WHERE team_id = $1 AND channel_id = $2 AND meta_template_id = $3 AND deleted_at IS NULL
	`
	return scanTemplate(r.db.Pool.QueryRow(ctx, query, teamID, channelID, metaTemplateID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Template], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(` AND channel_id = $%d`, len(args))
	}
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}

	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM templates WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Template]{}, fmt.Errorf("count templates: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	listQ := fmt.Sprintf(`
		SELECT id, channel_id, team_id, waba_id, meta_template_id, name, language,
			category, status, preview, components, variable_count, last_sync_at, created_at, updated_at, deleted_at
		FROM templates WHERE %s ORDER BY name ASC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args))

	rows, err := r.db.Pool.Query(ctx, listQ, args...)
	if err != nil {
		return repository.Page[Template]{}, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	items := make([]Template, 0)
	for rows.Next() {
		t, err := scanTemplateRow(rows)
		if err != nil {
			return repository.Page[Template]{}, err
		}
		items = append(items, *t)
	}
	return repository.Page[Template]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) UpsertFromSync(ctx context.Context, t *Template) error {
	query := `
		INSERT INTO templates (
			id, channel_id, team_id, waba_id, meta_template_id, name, language,
			category, status, preview, components, variable_count, last_sync_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (channel_id, meta_template_id) WHERE deleted_at IS NULL DO UPDATE SET
			name = EXCLUDED.name, language = EXCLUDED.language, category = EXCLUDED.category,
			status = EXCLUDED.status, preview = EXCLUDED.preview, components = EXCLUDED.components,
			variable_count = EXCLUDED.variable_count, last_sync_at = EXCLUDED.last_sync_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query,
		t.ID, t.ChannelID, t.TeamID, t.WABAID, t.MetaTemplateID, t.Name, t.Language,
		t.Category, string(t.Status), t.Preview, t.Components, t.VariableCount, t.LastSyncAt,
		t.CreatedAt, t.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE templates SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`,
		id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteExceptMetaIDs(ctx context.Context, teamID, channelID uuid.UUID, keepMetaIDs []string) (int64, error) {
	if len(keepMetaIDs) == 0 {
		tag, err := r.db.Pool.Exec(ctx, `
			UPDATE templates SET deleted_at = NOW(), updated_at = NOW()
			WHERE team_id = $1 AND channel_id = $2 AND deleted_at IS NULL
		`, teamID, channelID)
		if err != nil {
			return 0, repository.MapError(err)
		}
		return tag.RowsAffected(), nil
	}
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE templates SET deleted_at = NOW(), updated_at = NOW()
		WHERE team_id = $1 AND channel_id = $2 AND deleted_at IS NULL
			AND meta_template_id <> ALL($3::text[])
	`, teamID, channelID, keepMetaIDs)
	if err != nil {
		return 0, repository.MapError(err)
	}
	return tag.RowsAffected(), nil
}

func (r *PostgresRepository) HasRecentSync(ctx context.Context, teamID uuid.UUID, within time.Duration) (bool, error) {
	since := time.Now().UTC().Add(-within)
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM templates
			WHERE team_id = $1 AND deleted_at IS NULL AND last_sync_at >= $2
		)
	`, teamID, since).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has recent template sync: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) UpdateFromWebhook(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID, name string, status Status, category *string, updatedAt time.Time) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE templates SET
			name = COALESCE(NULLIF($4, ''), name),
			status = $5,
			category = COALESCE($6, category),
			updated_at = $7
		WHERE team_id = $1 AND channel_id = $2 AND meta_template_id = $3 AND deleted_at IS NULL
	`, teamID, channelID, metaTemplateID, name, string(status), category, updatedAt)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanTemplate(row pgx.Row) (*Template, error) {
	return scanTemplateRow(row)
}

func scanTemplateRow(row pgx.Row) (*Template, error) {
	var t Template
	var status string
	err := row.Scan(
		&t.ID, &t.ChannelID, &t.TeamID, &t.WABAID, &t.MetaTemplateID, &t.Name, &t.Language,
		&t.Category, &status, &t.Preview, &t.Components, &t.VariableCount, &t.LastSyncAt,
		&t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan template: %w", err)
	}
	t.Status = Status(status)
	return &t, nil
}
