package tag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (TagRepository) persists tags and contact assignments.
type Repository interface {
	Create(ctx context.Context, tag *Tag) error
	FindByID(ctx context.Context, teamID, tagID uuid.UUID) (*Tag, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Tag], error)
	Update(ctx context.Context, tag *Tag) error
	SoftDelete(ctx context.Context, teamID, tagID uuid.UUID) error
	AssignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error
	UnassignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error
	ListContactIDsByTag(ctx context.Context, teamID, tagID uuid.UUID) ([]uuid.UUID, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t *Tag) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO tags (id, team_id, name, color, description, contact_count, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, t.ID, t.TeamID, t.Name, t.Color, t.Description, t.ContactCount, t.CreatedBy, t.CreatedAt, t.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, tagID uuid.UUID) (*Tag, error) {
	return scanTag(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, name, color, description, contact_count, created_by, created_at, updated_at, deleted_at
		FROM tags WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, tagID, teamID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Tag], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}

	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM tags WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Tag]{}, fmt.Errorf("count tags: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, name, color, description, contact_count, created_by, created_at, updated_at, deleted_at
		FROM tags WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Tag]{}, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	items := make([]Tag, 0)
	for rows.Next() {
		t, err := scanTagRow(rows)
		if err != nil {
			return repository.Page[Tag]{}, err
		}
		items = append(items, *t)
	}
	return repository.Page[Tag]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, t *Tag) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE tags SET name = $1, color = $2, description = $3, updated_at = $4
		WHERE id = $5 AND team_id = $6 AND deleted_at IS NULL
	`, t.Name, t.Color, t.Description, t.UpdatedAt, t.ID, t.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, tagID uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE tags SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, tagID, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AssignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM tags t
				INNER JOIN contacts c ON c.team_id = t.team_id AND c.id = $3 AND c.deleted_at IS NULL
				WHERE t.id = $1 AND t.team_id = $2 AND t.deleted_at IS NULL
			)
		`, tagID, teamID, contactID).Scan(&exists); err != nil {
			return repository.MapError(err)
		}
		if !exists {
			return repository.ErrNotFound
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_tags (contact_id, tag_id, created_at)
			VALUES ($1,$2,NOW()) ON CONFLICT DO NOTHING
		`, contactID, tagID)
		if err != nil {
			return repository.MapError(err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE tags SET contact_count = (
				SELECT COUNT(*)::int
				FROM contact_tags ct
				INNER JOIN contacts c ON c.id = ct.contact_id AND c.deleted_at IS NULL
				WHERE ct.tag_id = $1
			), updated_at = NOW()
			WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
		`, tagID, teamID)
		return repository.MapError(err)
	})
}

func (r *PostgresRepository) UnassignContact(ctx context.Context, teamID, tagID, contactID uuid.UUID) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM contact_tags ct
			USING tags t
			WHERE ct.tag_id = t.id AND t.team_id = $1 AND ct.tag_id = $2 AND ct.contact_id = $3 AND t.deleted_at IS NULL
		`, teamID, tagID, contactID)
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() == 0 {
			return repository.ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			UPDATE tags SET contact_count = (
				SELECT COUNT(*)::int
				FROM contact_tags ct
				INNER JOIN contacts c ON c.id = ct.contact_id AND c.deleted_at IS NULL
				WHERE ct.tag_id = $1
			), updated_at = NOW()
			WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
		`, tagID, teamID)
		return repository.MapError(err)
	})
}

func (r *PostgresRepository) ListContactIDsByTag(ctx context.Context, teamID, tagID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT c.id FROM contacts c
		INNER JOIN contact_tags ct ON ct.contact_id = c.id
		INNER JOIN tags t ON t.id = ct.tag_id AND t.team_id = $1 AND t.deleted_at IS NULL
		WHERE ct.tag_id = $2 AND c.team_id = $1 AND c.deleted_at IS NULL
	`, teamID, tagID)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanTag(row pgx.Row) (*Tag, error) {
	var t Tag
	err := row.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.Description, &t.ContactCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt)
	if err != nil {
		return nil, repository.MapError(err)
	}
	return &t, nil
}

func scanTagRow(rows pgx.Rows) (*Tag, error) {
	var t Tag
	err := rows.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.Description, &t.ContactCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt)
	if err != nil {
		return nil, repository.MapError(err)
	}
	return &t, nil
}
