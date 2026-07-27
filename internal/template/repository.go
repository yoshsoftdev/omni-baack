package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (TemplateRepository) is the only persistence boundary for Template domain rows.
// Callers outside this package MUST NOT query the templates table directly.
type Repository interface {
	// FindByID loads a template by workspace + id.
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Template, error)
	// FindByIDs batch-loads templates by id (missing ids omitted).
	FindByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*Template, error)
	// FindByName loads by channel + name + language (unique active index).
	FindByName(ctx context.Context, teamID, channelID uuid.UUID, name, language string) (*Template, error)
	// FindByMetaID loads by Meta Graph template id within a channel.
	FindByMetaID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error)
	// FindByMetaTemplateID is an alias of FindByMetaID (legacy name).
	FindByMetaTemplateID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error)
	// List returns a page of domain Templates.
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Template], error)
	// Save inserts a new Template (Create). Prefer Update for existing rows.
	Save(ctx context.Context, t *Template) error
	// Create is an alias of Save.
	Create(ctx context.Context, t *Template) error
	// Update replaces mutable fields of an existing Template by id + workspace.
	Update(ctx context.Context, t *Template) error
	// Delete soft-deletes a Template.
	Delete(ctx context.Context, teamID, id uuid.UUID) error
	// SoftDelete is an alias of Delete.
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	// UpsertFromSync persists Meta sync results (preserves metadata on conflict).
	UpsertFromSync(ctx context.Context, t *Template) error
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

const templateSelectCols = `
	id, channel_id, team_id, waba_id, meta_template_id, name, language,
	normalized_language, category, status, preview, components, variable_count,
	metadata, sync_version, last_sync_at, created_at, updated_at, deleted_at
`

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Template, error) {
	query := `SELECT ` + templateSelectCols + `
		FROM templates WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`
	return scanTemplate(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresRepository) FindByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*Template, error) {
	out := make(map[uuid.UUID]*Template, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+templateSelectCols+`
		FROM templates
		WHERE team_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`, teamID, ids)
	if err != nil {
		return nil, fmt.Errorf("find templates by ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		out[t.ID] = t
	}
	return out, rows.Err()
}

func (r *PostgresRepository) FindByName(ctx context.Context, teamID, channelID uuid.UUID, name, language string) (*Template, error) {
	name = strings.TrimSpace(name)
	language = strings.TrimSpace(language)
	query := `SELECT ` + templateSelectCols + `
		FROM templates
		WHERE team_id = $1 AND channel_id = $2 AND name = $3 AND language = $4 AND deleted_at IS NULL`
	return scanTemplate(r.db.Pool.QueryRow(ctx, query, teamID, channelID, name, language))
}

func (r *PostgresRepository) FindByMetaID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error) {
	return r.FindByMetaTemplateID(ctx, teamID, channelID, metaTemplateID)
}

func (r *PostgresRepository) FindByMetaTemplateID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*Template, error) {
	query := `SELECT ` + templateSelectCols + `
		FROM templates
		WHERE team_id = $1 AND channel_id = $2 AND meta_template_id = $3 AND deleted_at IS NULL`
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
	if filter.Language != "" {
		args = append(args, filter.Language)
		where += fmt.Sprintf(` AND normalized_language = $%d`, len(args))
	}
	if filter.Category != "" {
		args = append(args, filter.Category)
		where += fmt.Sprintf(` AND category = $%d::template_category`, len(args))
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
		SELECT %s
		FROM templates WHERE %s ORDER BY name ASC LIMIT $%d OFFSET $%d
	`, templateSelectCols, where, len(args)-1, len(args))

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

func (r *PostgresRepository) Save(ctx context.Context, t *Template) error {
	return r.Create(ctx, t)
}

func (r *PostgresRepository) Create(ctx context.Context, t *Template) error {
	ensureDomainDefaults(t)
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	query := `
		INSERT INTO templates (
			id, channel_id, team_id, waba_id, meta_template_id, name, language,
			normalized_language, category, status, preview, components, variable_count,
			metadata, sync_version, last_sync_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		t.ID, t.ChannelID, t.TeamID, t.WABAID, t.MetaTemplateID, t.Name, t.Language,
		t.NormalizedLanguage, t.Category, string(t.Status), t.Preview, t.Components, t.VariableCount,
		t.Metadata.Raw(), t.SyncVersion, t.LastSyncAt, t.CreatedAt, t.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) Update(ctx context.Context, t *Template) error {
	if t == nil || t.ID == uuid.Nil {
		return repository.ErrNotFound
	}
	ensureDomainDefaults(t)
	t.UpdatedAt = time.Now().UTC()
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE templates SET
			name = $3,
			language = $4,
			normalized_language = $5,
			category = $6,
			status = $7,
			preview = $8,
			components = $9,
			variable_count = $10,
			metadata = $11,
			sync_version = $12,
			last_sync_at = $13,
			waba_id = $14,
			meta_template_id = $15,
			updated_at = $16
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`,
		t.ID, t.TeamID,
		t.Name, t.Language, t.NormalizedLanguage, t.Category, string(t.Status), t.Preview,
		t.Components, t.VariableCount, t.Metadata.Raw(), t.SyncVersion, t.LastSyncAt,
		t.WABAID, t.MetaTemplateID, t.UpdatedAt,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, teamID, id uuid.UUID) error {
	return r.SoftDelete(ctx, teamID, id)
}

func (r *PostgresRepository) UpsertFromSync(ctx context.Context, t *Template) error {
	ensureDomainDefaults(t)
	// Components/metadata must already be merge-prepared by SyncService (MergeSyncedTemplate).
	// On conflict, write EXCLUDED.metadata (merged domain value), never wipe with empty Meta payload.
	query := `
		INSERT INTO templates (
			id, channel_id, team_id, waba_id, meta_template_id, name, language,
			normalized_language, category, status, preview, components, variable_count,
			metadata, sync_version, last_sync_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (channel_id, meta_template_id) WHERE deleted_at IS NULL DO UPDATE SET
			name = EXCLUDED.name,
			language = EXCLUDED.language,
			normalized_language = EXCLUDED.normalized_language,
			category = EXCLUDED.category,
			status = EXCLUDED.status,
			preview = EXCLUDED.preview,
			components = EXCLUDED.components,
			variable_count = EXCLUDED.variable_count,
			metadata = CASE
				WHEN EXCLUDED.metadata IS NULL OR EXCLUDED.metadata = '{}'::jsonb THEN templates.metadata
				ELSE EXCLUDED.metadata
			END,
			sync_version = templates.sync_version + 1,
			last_sync_at = EXCLUDED.last_sync_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query,
		t.ID, t.ChannelID, t.TeamID, t.WABAID, t.MetaTemplateID, t.Name, t.Language,
		t.NormalizedLanguage, t.Category, string(t.Status), t.Preview, t.Components, t.VariableCount,
		t.Metadata.Raw(), t.SyncVersion, t.LastSyncAt, t.CreatedAt, t.UpdatedAt,
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

func ensureDomainDefaults(t *Template) {
	if t == nil {
		return
	}
	t.Metadata = t.Metadata.Normalize()
	if t.Language != "" {
		// Keep stored language as-is if already set; only fill normalized key.
		t.NormalizedLanguage = DefaultLanguageResolver().NormalizedStorageKey(t.Language)
	} else if t.NormalizedLanguage == "" {
		t.NormalizedLanguage = t.Language
	}
	if len(t.Components) == 0 {
		t.Components = jsonEmptyArray()
	}
}

func jsonEmptyArray() []byte {
	return []byte("[]")
}

func scanTemplate(row pgx.Row) (*Template, error) {
	return scanTemplateRow(row)
}

func scanTemplateRow(row pgx.Row) (*Template, error) {
	var t Template
	var status string
	var metadata []byte
	err := row.Scan(
		&t.ID, &t.ChannelID, &t.TeamID, &t.WABAID, &t.MetaTemplateID, &t.Name, &t.Language,
		&t.NormalizedLanguage, &t.Category, &status, &t.Preview, &t.Components, &t.VariableCount,
		&metadata, &t.SyncVersion, &t.LastSyncAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan template: %w", err)
	}
	t.Status = Status(status)
	t.Metadata = Metadata(metadata).Normalize()
	return &t, nil
}
