package quickreply

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type Repository interface {
	Create(ctx context.Context, item *QuickReply) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*QuickReply, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[QuickReply], error)
	Update(ctx context.Context, item *QuickReply) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, item *QuickReply) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO quick_replies (id, team_id, category, content, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, item.ID, item.TeamID, item.Category, item.Content, item.CreatedBy, item.CreatedAt, item.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*QuickReply, error) {
	return scanQuickReply(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, category, content, created_by, created_at, updated_at, deleted_at
		FROM quick_replies
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[QuickReply], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND (category ILIKE $%d OR content ILIKE $%d)`, len(args), len(args))
	}

	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM quick_replies WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[QuickReply]{}, fmt.Errorf("count quick replies: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, category, content, created_by, created_at, updated_at, deleted_at
		FROM quick_replies
		WHERE %s
		ORDER BY category ASC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[QuickReply]{}, fmt.Errorf("list quick replies: %w", err)
	}
	defer rows.Close()

	items := make([]QuickReply, 0)
	for rows.Next() {
		item, err := scanQuickReplyRow(rows)
		if err != nil {
			return repository.Page[QuickReply]{}, err
		}
		items = append(items, *item)
	}
	return repository.Page[QuickReply]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, item *QuickReply) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE quick_replies
		SET category = $1, content = $2, updated_at = $3
		WHERE id = $4 AND team_id = $5 AND deleted_at IS NULL
	`, item.Category, item.Content, item.UpdatedAt, item.ID, item.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE quick_replies
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanQuickReply(row pgx.Row) (*QuickReply, error) {
	var item QuickReply
	err := row.Scan(&item.ID, &item.TeamID, &item.Category, &item.Content, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		return nil, repository.MapError(err)
	}
	item.Text = item.Content
	return &item, nil
}

func scanQuickReplyRow(rows pgx.Rows) (*QuickReply, error) {
	var item QuickReply
	err := rows.Scan(&item.ID, &item.TeamID, &item.Category, &item.Content, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		return nil, repository.MapError(err)
	}
	item.Text = item.Content
	return &item, nil
}
