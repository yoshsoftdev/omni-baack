package contact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (ContactRepository) persists contacts.
type Repository interface {
	Create(ctx context.Context, c *Contact) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Contact, error)
	ResolveInstagramScopedID(ctx context.Context, teamID, contactID uuid.UUID) (string, error)
	FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*Contact, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Contact], error)
	ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Contact, error)
	ListTagsByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]TagRef, error)
	SyncContactTags(ctx context.Context, teamID, contactID uuid.UUID, tagIDs []uuid.UUID) error
	Update(ctx context.Context, c *Contact) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error)
	CountMarketingByTeam(ctx context.Context, teamID uuid.UUID) (map[MarketingStatus]int64, error)
	LoadContactStats(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]ContactStats, error)
	ListFieldDefinitions(ctx context.Context, teamID uuid.UUID) ([]FieldDefinition, error)
	FindFieldDefinitionByID(ctx context.Context, teamID, id uuid.UUID) (*FieldDefinition, error)
	CreateFieldDefinition(ctx context.Context, def *FieldDefinition) error
	UpdateFieldDefinition(ctx context.Context, def *FieldDefinition) error
	SoftDeleteFieldDefinition(ctx context.Context, teamID, id uuid.UUID) error
	ListCustomFieldsByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]CustomFieldValue, error)
	UpsertCustomFieldValues(ctx context.Context, contactID uuid.UUID, values map[uuid.UUID]json.RawMessage) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, c *Contact) error {
	query := `
		INSERT INTO contacts (
			id, team_id, contact_name, customer_name, phone, email, avatar_url, notes, city, country, province, company, job_title,
			language, customer_type, source, marketing_status, last_activity_at, import_job_id, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		c.ID, c.TeamID, c.ContactName, c.CustomerName, nullableContactPhone(c.Phone), c.Email, c.AvatarURL, c.Notes, c.City, c.Country, c.Province, c.Company, c.JobTitle,
		c.Language, c.CustomerType, string(c.Source), string(c.MarketingStatus), c.LastActivityAt, c.ImportJobID, c.CreatedAt, c.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) ResolveInstagramScopedID(ctx context.Context, teamID, contactID uuid.UUID) (string, error) {
	// Prefer provider_profiles: linked contacts keep WhatsApp phone on contacts.phone,
	// so stripping a non-ig phone must never win over the Instagram IGSID.
	query := `
		SELECT COALESCE(
			(
				SELECT NULLIF(TRIM(p.provider_user_id), '')
				FROM provider_profiles p
				WHERE p.team_id = c.team_id
				  AND p.contact_id = c.id
				  AND p.provider = 'instagram'
				  AND p.deleted_at IS NULL
				ORDER BY p.updated_at DESC
				LIMIT 1
			),
			CASE
				WHEN c.phone LIKE 'ig:%' THEN NULLIF(TRIM(BOTH FROM regexp_replace(c.phone, '^ig:', '')), '')
				ELSE NULL
			END,
			''
		)
		FROM contacts c
		WHERE c.id = $1 AND c.team_id = $2 AND c.deleted_at IS NULL
	`
	var scopedID string
	if err := r.db.Pool.QueryRow(ctx, query, contactID, teamID).Scan(&scopedID); err != nil {
		return "", repository.MapError(err)
	}
	scopedID = strings.TrimSpace(scopedID)
	if scopedID == "" {
		return "", repository.ErrNotFound
	}
	return scopedID, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Contact, error) {
	query := `
		SELECT id, team_id, contact_name, customer_name, COALESCE(phone, ''), email, avatar_url, notes, city, country, province, company, job_title,
			language, customer_type, source, marketing_status, last_activity_at, import_job_id, created_at, updated_at, deleted_at
		FROM contacts WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return scanContact(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresRepository) FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*Contact, error) {
	query := `
		SELECT id, team_id, contact_name, customer_name, COALESCE(phone, ''), email, avatar_url, notes, city, country, province, company, job_title,
			language, customer_type, source, marketing_status, last_activity_at, import_job_id, created_at, updated_at, deleted_at
		FROM contacts WHERE team_id = $1 AND phone = $2 AND deleted_at IS NULL
	`
	return scanContact(r.db.Pool.QueryRow(ctx, query, teamID, phone))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Contact], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `c.team_id = $1 AND c.deleted_at IS NULL`
	joins := ""

	if filter.TagID != nil {
		joins += ` INNER JOIN contact_tags ct ON ct.contact_id = c.id`
		args = append(args, *filter.TagID)
		where += fmt.Sprintf(` AND ct.tag_id = $%d`, len(args))
	}
	if filter.SegmentID != nil {
		joins += ` INNER JOIN segment_members sm ON sm.contact_id = c.id`
		args = append(args, *filter.SegmentID)
		where += fmt.Sprintf(` AND sm.segment_id = $%d`, len(args))
	}
	if filter.MarketingStatus != nil {
		args = append(args, string(*filter.MarketingStatus))
		where += fmt.Sprintf(` AND c.marketing_status = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND (
			c.contact_name ILIKE $%d OR c.customer_name ILIKE $%d OR c.phone ILIKE $%d OR c.email ILIKE $%d
			OR EXISTS (
				SELECT 1 FROM provider_profiles p
				WHERE p.team_id = c.team_id AND p.contact_id = c.id AND p.deleted_at IS NULL
				  AND (p.display_name ILIKE $%d OR p.username ILIKE $%d OR p.phone ILIKE $%d OR p.email ILIKE $%d)
			)
		)`, len(args), len(args), len(args), len(args), len(args), len(args), len(args), len(args))
	}

	countQ := `SELECT COUNT(DISTINCT c.id) FROM contacts c` + joins + ` WHERE ` + where
	var total int64
	if err := r.db.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return repository.Page[Contact]{}, fmt.Errorf("count contacts: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	listQ := fmt.Sprintf(`
		SELECT DISTINCT c.id, c.team_id, c.contact_name, c.customer_name, COALESCE(c.phone, ''), c.email, c.avatar_url, c.notes, c.city, c.country, c.province, c.company, c.job_title,
			c.language, c.customer_type, c.source, c.marketing_status, c.last_activity_at, c.import_job_id, c.created_at, c.updated_at, c.deleted_at
		FROM contacts c%s WHERE %s ORDER BY c.created_at DESC LIMIT $%d OFFSET $%d
	`, joins, where, len(args)-1, len(args))

	rows, err := r.db.Pool.Query(ctx, listQ, args...)
	if err != nil {
		return repository.Page[Contact]{}, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	items := make([]Contact, 0)
	for rows.Next() {
		c, err := scanContactRow(rows)
		if err != nil {
			return repository.Page[Contact]{}, err
		}
		items = append(items, *c)
	}
	return repository.Page[Contact]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Contact, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, team_id, contact_name, customer_name, COALESCE(phone, ''), email, avatar_url, notes, city, country, province, company, job_title,
			language, customer_type, source, marketing_status, last_activity_at, import_job_id, created_at, updated_at, deleted_at
		FROM contacts WHERE team_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`, teamID, ids)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	out := make([]Contact, 0, len(ids))
	for rows.Next() {
		c, err := scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListTagsByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]TagRef, error) {
	out := make(map[uuid.UUID][]TagRef)
	if len(contactIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT ct.contact_id, t.id, t.name, t.color
		FROM contact_tags ct
		INNER JOIN tags t ON t.id = ct.tag_id AND t.team_id = $1 AND t.deleted_at IS NULL
		INNER JOIN contacts c ON c.id = ct.contact_id AND c.team_id = $1 AND c.deleted_at IS NULL
		WHERE ct.contact_id = ANY($2)
		ORDER BY t.name ASC
	`, teamID, contactIDs)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var contactID uuid.UUID
		var tag TagRef
		if err := rows.Scan(&contactID, &tag.ID, &tag.Name, &tag.Color); err != nil {
			return nil, err
		}
		out[contactID] = append(out[contactID], tag)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, c *Contact) error {
	query := `
		UPDATE contacts SET
			contact_name = $1, customer_name = $2, phone = $3, email = $4, avatar_url = $5, notes = $6, city = $7, country = $8, province = $9,
			company = $10, job_title = $11, language = $12, customer_type = $13, source = $14,
			marketing_status = $15, last_activity_at = $16, updated_at = $17
		WHERE id = $18 AND team_id = $19 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		c.ContactName, c.CustomerName, nullableContactPhone(c.Phone), c.Email, c.AvatarURL, c.Notes, c.City, c.Country, c.Province, c.Company, c.JobTitle,
		c.Language, c.CustomerType, string(c.Source), string(c.MarketingStatus), c.LastActivityAt, c.UpdatedAt, c.ID, c.TeamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE contacts SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`,
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

func (r *PostgresRepository) CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM contacts WHERE team_id = $1 AND deleted_at IS NULL`, teamID,
	).Scan(&n)
	return n, err
}

func scanContact(row pgx.Row) (*Contact, error) {
	return scanContactRow(row)
}

func scanContactRow(row pgx.Row) (*Contact, error) {
	var c Contact
	var source, marketing string
	err := row.Scan(
		&c.ID, &c.TeamID, &c.ContactName, &c.CustomerName, &c.Phone, &c.Email, &c.AvatarURL, &c.Notes, &c.City, &c.Country, &c.Province, &c.Company, &c.JobTitle,
		&c.Language, &c.CustomerType, &source, &marketing, &c.LastActivityAt, &c.ImportJobID, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan contact: %w", err)
	}
	c.Source = Source(source)
	c.MarketingStatus = MarketingStatus(marketing)
	return &c, nil
}

func nullableContactPhone(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
