package segment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (SegmentRepository) persists segments, rules, and members.
type Repository interface {
	Create(ctx context.Context, s *Segment, rules []Rule) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Segment, []Rule, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Segment], error)
	Update(ctx context.Context, s *Segment, rules []Rule) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error

	ReplaceMembers(ctx context.Context, segmentID uuid.UUID, contactIDs []uuid.UUID) error
	ListMemberContactIDs(ctx context.Context, segmentID uuid.UUID, page repository.PageParams) ([]uuid.UUID, int64, error)
	CountMembers(ctx context.Context, segmentID uuid.UUID) (int64, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, s *Segment, rules []Rule) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO segments (id, team_id, name, description, color, contact_count, is_dynamic, last_evaluated_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, s.ID, s.TeamID, s.Name, s.Description, s.Color, s.ContactCount, s.IsDynamic, s.LastEvaluatedAt, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return repository.MapError(err)
		}
		for _, rule := range rules {
			_, err := tx.Exec(ctx, `
				INSERT INTO segment_rules (id, segment_id, field, operator, value, sort_order, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7)
			`, rule.ID, s.ID, string(rule.Field), string(rule.Operator), rule.Value, rule.SortOrder, rule.CreatedAt)
			if err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Segment, []Rule, error) {
	s, err := scanSegment(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, name, description, color, contact_count, is_dynamic, last_evaluated_at, created_at, updated_at, deleted_at
		FROM segments WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
	if err != nil {
		return nil, nil, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, segment_id, field, operator, value, sort_order, created_at
		FROM segment_rules WHERE segment_id = $1 ORDER BY sort_order ASC
	`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, nil, err
		}
		rules = append(rules, *rule)
	}
	return s, rules, rows.Err()
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Segment], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM segments WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Segment]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, name, description, color, contact_count, is_dynamic, last_evaluated_at, created_at, updated_at, deleted_at
		FROM segments WHERE %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Segment]{}, err
	}
	defer rows.Close()
	items := make([]Segment, 0)
	for rows.Next() {
		s, err := scanSegmentRow(rows)
		if err != nil {
			return repository.Page[Segment]{}, err
		}
		items = append(items, *s)
	}
	return repository.Page[Segment]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, s *Segment, rules []Rule) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE segments SET name=$1, description=$2, color=$3, contact_count=$4, is_dynamic=$5, last_evaluated_at=$6, updated_at=$7
			WHERE id=$8 AND team_id=$9 AND deleted_at IS NULL
		`, s.Name, s.Description, s.Color, s.ContactCount, s.IsDynamic, s.LastEvaluatedAt, s.UpdatedAt, s.ID, s.TeamID)
		if err != nil {
			return repository.MapError(err)
		}
		if tag.RowsAffected() == 0 {
			return repository.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM segment_rules WHERE segment_id = $1`, s.ID); err != nil {
			return err
		}
		for _, rule := range rules {
			_, err := tx.Exec(ctx, `
				INSERT INTO segment_rules (id, segment_id, field, operator, value, sort_order, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7)
			`, rule.ID, s.ID, string(rule.Field), string(rule.Operator), rule.Value, rule.SortOrder, rule.CreatedAt)
			if err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE segments SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`,
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

func (r *PostgresRepository) ReplaceMembers(ctx context.Context, segmentID uuid.UUID, contactIDs []uuid.UUID) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM segment_members WHERE segment_id = $1`, segmentID); err != nil {
			return err
		}
		for _, cid := range contactIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO segment_members (segment_id, contact_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				segmentID, cid,
			); err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) ListMemberContactIDs(ctx context.Context, segmentID uuid.UUID, page repository.PageParams) ([]uuid.UUID, int64, error) {
	page = page.Normalize()
	var total int64
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_members WHERE segment_id = $1`, segmentID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Pool.Query(ctx,
		`SELECT contact_id FROM segment_members WHERE segment_id = $1 ORDER BY added_at ASC LIMIT $2 OFFSET $3`,
		segmentID, page.Limit(), page.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

func (r *PostgresRepository) CountMembers(ctx context.Context, segmentID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM segment_members WHERE segment_id = $1`, segmentID).Scan(&n)
	return n, err
}

func scanSegment(row pgx.Row) (*Segment, error) { return scanSegmentRow(row) }

func scanSegmentRow(row pgx.Row) (*Segment, error) {
	var s Segment
	err := row.Scan(&s.ID, &s.TeamID, &s.Name, &s.Description, &s.Color, &s.ContactCount, &s.IsDynamic,
		&s.LastEvaluatedAt, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan segment: %w", err)
	}
	return &s, nil
}

func scanRule(row pgx.Row) (*Rule, error) {
	var rule Rule
	var field, op string
	err := row.Scan(&rule.ID, &rule.SegmentID, &field, &op, &rule.Value, &rule.SortOrder, &rule.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan rule: %w", err)
	}
	rule.Field = RuleField(field)
	rule.Operator = RuleOperator(op)
	return &rule, nil
}
