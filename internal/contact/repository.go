package contact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("contact not found")

type ListFilter struct {
	Search string
	TagID  *uuid.UUID
	Limit  int
	Offset int
}

type Repository interface {
	Create(ctx context.Context, contact *Contact) error
	FindByID(ctx context.Context, teamID, contactID uuid.UUID) (*Contact, error)
	FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*Contact, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]Contact, error)
	Update(ctx context.Context, contact *Contact) error
	SoftDelete(ctx context.Context, teamID, contactID uuid.UUID) error
	ListTags(ctx context.Context, contactID uuid.UUID) ([]tag.Tag, error)
	SetTags(ctx context.Context, contactID uuid.UUID, tagIDs []uuid.UUID) error
	ListByTagIDs(ctx context.Context, teamID uuid.UUID, tagIDs []uuid.UUID) ([]Contact, error)
	ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Contact, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, contact *Contact) error {
	query := `
		INSERT INTO contacts (id, team_id, name, phone, email, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		contact.ID, contact.TeamID, contact.Name, contact.Phone,
		contact.Email, contact.Notes, contact.CreatedAt, contact.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("phone already exists")
		}
		return fmt.Errorf("insert contact: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, contactID uuid.UUID) (*Contact, error) {
	query := `
		SELECT id, team_id, name, phone, email, notes, created_at, updated_at, deleted_at
		FROM contacts
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, contactID, teamID))
}

func (r *PostgresRepository) FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*Contact, error) {
	query := `
		SELECT id, team_id, name, phone, email, notes, created_at, updated_at, deleted_at
		FROM contacts
		WHERE team_id = $1 AND phone = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, teamID, phone))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID, filter ListFilter) ([]Contact, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	args := []any{teamID}
	where := "c.team_id = $1 AND c.deleted_at IS NULL"
	argPos := 2

	if filter.Search != "" {
		where += fmt.Sprintf(" AND (c.name ILIKE $%d OR c.phone ILIKE $%d)", argPos, argPos)
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}
	if filter.TagID != nil {
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM contact_tags ct
			WHERE ct.contact_id = c.id AND ct.tag_id = $%d
		)`, argPos)
		args = append(args, *filter.TagID)
		argPos++
	}

	query := fmt.Sprintf(`
		SELECT c.id, c.team_id, c.name, c.phone, c.email, c.notes, c.created_at, c.updated_at, c.deleted_at
		FROM contacts c
		WHERE %s
		ORDER BY c.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, limit, filter.Offset)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, *item)
	}
	return contacts, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, contact *Contact) error {
	query := `
		UPDATE contacts
		SET name = $1, phone = $2, email = $3, notes = $4, updated_at = $5
		WHERE id = $6 AND team_id = $7 AND deleted_at IS NULL
	`
	result, err := r.db.Pool.Exec(ctx, query,
		contact.Name, contact.Phone, contact.Email, contact.Notes,
		contact.UpdatedAt, contact.ID, contact.TeamID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("phone already exists")
		}
		return fmt.Errorf("update contact: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, contactID uuid.UUID) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM contact_tags WHERE contact_id = $1`, contactID)
	if err != nil {
		return fmt.Errorf("delete contact tags: %w", err)
	}

	result, err := tx.Exec(ctx, `
		UPDATE contacts SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, contactID, teamID)
	if err != nil {
		return fmt.Errorf("delete contact: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListTags(ctx context.Context, contactID uuid.UUID) ([]tag.Tag, error) {
	query := `
		SELECT t.id, t.team_id, t.name, t.color, t.created_at, t.updated_at, t.deleted_at
		FROM tags t
		INNER JOIN contact_tags ct ON ct.tag_id = t.id
		WHERE ct.contact_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, contactID)
	if err != nil {
		return nil, fmt.Errorf("list contact tags: %w", err)
	}
	defer rows.Close()

	var tags []tag.Tag
	for rows.Next() {
		var t tag.Tag
		var deletedAt *time.Time
		if err := rows.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("scan contact tag: %w", err)
		}
		t.DeletedAt = deletedAt
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (r *PostgresRepository) SetTags(ctx context.Context, contactID uuid.UUID, tagIDs []uuid.UUID) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM contact_tags WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("clear contact tags: %w", err)
	}

	for _, tagID := range tagIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_tags (contact_id, tag_id) VALUES ($1, $2)
		`, contactID, tagID)
		if err != nil {
			return fmt.Errorf("assign contact tag: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListByTagIDs(ctx context.Context, teamID uuid.UUID, tagIDs []uuid.UUID) ([]Contact, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	query := `
		SELECT DISTINCT c.id, c.team_id, c.name, c.phone, c.email, c.notes, c.created_at, c.updated_at, c.deleted_at
		FROM contacts c
		INNER JOIN contact_tags ct ON ct.contact_id = c.id
		WHERE c.team_id = $1 AND c.deleted_at IS NULL AND ct.tag_id = ANY($2)
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, tagIDs)
	if err != nil {
		return nil, fmt.Errorf("list contacts by tags: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, *item)
	}
	return contacts, rows.Err()
}

func (r *PostgresRepository) ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Contact, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `
		SELECT id, team_id, name, phone, email, notes, created_at, updated_at, deleted_at
		FROM contacts
		WHERE team_id = $1 AND deleted_at IS NULL AND id = ANY($2)
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, ids)
	if err != nil {
		return nil, fmt.Errorf("list contacts by ids: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, *item)
	}
	return contacts, rows.Err()
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Contact, error) {
	var c Contact
	var deletedAt *time.Time
	err := row.Scan(&c.ID, &c.TeamID, &c.Name, &c.Phone, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt, &deletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan contact: %w", err)
	}
	c.DeletedAt = deletedAt
	return &c, nil
}

func (r *PostgresRepository) scanRow(rows pgx.Rows) (*Contact, error) {
	var c Contact
	var deletedAt *time.Time
	err := rows.Scan(&c.ID, &c.TeamID, &c.Name, &c.Phone, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt, &deletedAt)
	if err != nil {
		return nil, fmt.Errorf("scan contact row: %w", err)
	}
	c.DeletedAt = deletedAt
	return &c, nil
}
