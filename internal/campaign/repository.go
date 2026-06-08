package campaign

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

var ErrNotFound = errors.New("campaign not found")

type Repository interface {
	Create(ctx context.Context, campaign *Campaign, contactIDs, tagIDs []uuid.UUID) error
	FindByID(ctx context.Context, teamID, campaignID uuid.UUID) (*Campaign, error)
	FindByIDOnly(ctx context.Context, campaignID uuid.UUID) (*Campaign, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Campaign, error)
	Update(ctx context.Context, campaign *Campaign) error
	ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error)
	ReplaceContacts(ctx context.Context, campaignID uuid.UUID, contacts []CampaignContact) error
	ListPendingContacts(ctx context.Context, campaignID uuid.UUID) ([]CampaignContact, error)
	UpdateContact(ctx context.Context, contact *CampaignContact) error
	IncrementCounts(ctx context.Context, campaignID uuid.UUID, sent, failed int) error
	FindContactByWamid(ctx context.Context, wamid string) (*CampaignContact, error)
	UpdateContactStatus(ctx context.Context, wamid string, status ContactStatus, at time.Time) error
	RecalculateCounts(ctx context.Context, campaignID uuid.UUID) error
	ListTagIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error)
	ListAudienceContactIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, campaign *Campaign, contactIDs, tagIDs []uuid.UUID) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO campaigns (
			id, team_id, device_id, template_id, name, status, variable_mapping,
			scheduled_at, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = tx.Exec(ctx, query,
		campaign.ID, campaign.TeamID, campaign.DeviceID, campaign.TemplateID,
		campaign.Name, string(campaign.Status), campaign.VariableMapping,
		campaign.ScheduledAt, campaign.CreatedBy, campaign.CreatedAt, campaign.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert campaign: %w", err)
	}

	for _, contactID := range contactIDs {
		_, err = tx.Exec(ctx, `INSERT INTO campaign_audience_contacts (campaign_id, contact_id) VALUES ($1, $2)`, campaign.ID, contactID)
		if err != nil {
			return fmt.Errorf("insert campaign audience contact: %w", err)
		}
	}

	for _, tagID := range tagIDs {
		_, err = tx.Exec(ctx, `INSERT INTO campaign_tags (campaign_id, tag_id) VALUES ($1, $2)`, campaign.ID, tagID)
		if err != nil {
			return fmt.Errorf("insert campaign tag: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, campaignID uuid.UUID) (*Campaign, error) {
	query := `
		SELECT id, team_id, device_id, template_id, name, status, variable_mapping,
			scheduled_at, started_at, completed_at, created_by,
			total_contacts, sent_count, delivered_count, read_count, failed_count,
			created_at, updated_at, deleted_at
		FROM campaigns
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanCampaign(r.db.Pool.QueryRow(ctx, query, campaignID, teamID))
}

func (r *PostgresRepository) FindByIDOnly(ctx context.Context, campaignID uuid.UUID) (*Campaign, error) {
	query := `
		SELECT id, team_id, device_id, template_id, name, status, variable_mapping,
			scheduled_at, started_at, completed_at, created_by,
			total_contacts, sent_count, delivered_count, read_count, failed_count,
			created_at, updated_at, deleted_at
		FROM campaigns
		WHERE id = $1 AND deleted_at IS NULL
	`
	return r.scanCampaign(r.db.Pool.QueryRow(ctx, query, campaignID))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Campaign, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT id, team_id, device_id, template_id, name, status, variable_mapping,
			scheduled_at, started_at, completed_at, created_by,
			total_contacts, sent_count, delivered_count, read_count, failed_count,
			created_at, updated_at, deleted_at
		FROM campaigns
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Pool.Query(ctx, query, teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()

	var items []Campaign
	for rows.Next() {
		item, err := r.scanCampaignRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, campaign *Campaign) error {
	query := `
		UPDATE campaigns SET
			status = $3,
			scheduled_at = $4,
			started_at = $5,
			completed_at = $6,
			total_contacts = $7,
			sent_count = $8,
			delivered_count = $9,
			read_count = $10,
			failed_count = $11,
			updated_at = $12
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	_, err := r.db.Pool.Exec(ctx, query,
		campaign.ID, campaign.TeamID, string(campaign.Status),
		campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt,
		campaign.TotalContacts, campaign.SentCount, campaign.DeliveredCount,
		campaign.ReadCount, campaign.FailedCount, campaign.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update campaign: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, team_id, device_id, template_id, name, status, variable_mapping,
			scheduled_at, started_at, completed_at, created_by,
			total_contacts, sent_count, delivered_count, read_count, failed_count,
			created_at, updated_at, deleted_at
		FROM campaigns
		WHERE status = 'scheduled' AND scheduled_at <= $1 AND deleted_at IS NULL
		ORDER BY scheduled_at ASC
		LIMIT $2
	`
	rows, err := r.db.Pool.Query(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due campaigns: %w", err)
	}
	defer rows.Close()

	var items []Campaign
	for rows.Next() {
		item, err := r.scanCampaignRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) ReplaceContacts(ctx context.Context, campaignID uuid.UUID, contacts []CampaignContact) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM campaign_contacts WHERE campaign_id = $1`, campaignID); err != nil {
		return fmt.Errorf("delete campaign contacts: %w", err)
	}

	for i := range contacts {
		cc := &contacts[i]
		query := `
			INSERT INTO campaign_contacts (id, campaign_id, contact_id, phone, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`
		_, err = tx.Exec(ctx, query, cc.ID, cc.CampaignID, cc.ContactID, cc.Phone, string(cc.Status), cc.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert campaign contact: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListPendingContacts(ctx context.Context, campaignID uuid.UUID) ([]CampaignContact, error) {
	query := `
		SELECT id, campaign_id, contact_id, phone, status, wamid, error_message,
			sent_at, delivered_at, read_at, created_at
		FROM campaign_contacts
		WHERE campaign_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list pending contacts: %w", err)
	}
	defer rows.Close()

	var items []CampaignContact
	for rows.Next() {
		item, err := r.scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) UpdateContact(ctx context.Context, contact *CampaignContact) error {
	query := `
		UPDATE campaign_contacts SET
			status = $2, wamid = $3, error_message = $4, sent_at = $5
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query,
		contact.ID, string(contact.Status), contact.Wamid, contact.ErrorMessage, contact.SentAt,
	)
	if err != nil {
		return fmt.Errorf("update campaign contact: %w", err)
	}
	return nil
}

func (r *PostgresRepository) IncrementCounts(ctx context.Context, campaignID uuid.UUID, sent, failed int) error {
	query := `
		UPDATE campaigns SET
			sent_count = sent_count + $2,
			failed_count = failed_count + $3,
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, campaignID, sent, failed)
	return err
}

func (r *PostgresRepository) FindContactByWamid(ctx context.Context, wamid string) (*CampaignContact, error) {
	query := `
		SELECT id, campaign_id, contact_id, phone, status, wamid, error_message,
			sent_at, delivered_at, read_at, created_at
		FROM campaign_contacts
		WHERE wamid = $1
		LIMIT 1
	`
	return r.scanContact(r.db.Pool.QueryRow(ctx, query, wamid))
}

func (r *PostgresRepository) UpdateContactStatus(ctx context.Context, wamid string, status ContactStatus, at time.Time) error {
	var query string
	switch status {
	case ContactDelivered:
		query = `UPDATE campaign_contacts SET status = $2, delivered_at = $3 WHERE wamid = $1 AND status IN ('sent', 'pending')`
	case ContactRead:
		query = `UPDATE campaign_contacts SET status = $2, read_at = $3 WHERE wamid = $1`
	default:
		query = `UPDATE campaign_contacts SET status = $2 WHERE wamid = $1`
	}
	_, err := r.db.Pool.Exec(ctx, query, wamid, string(status), at)
	return err
}

func (r *PostgresRepository) ListAudienceContactIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT contact_id FROM campaign_audience_contacts WHERE campaign_id = $1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list campaign audience contacts: %w", err)
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

func (r *PostgresRepository) ListTagIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT tag_id FROM campaign_tags WHERE campaign_id = $1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list campaign tags: %w", err)
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

func (r *PostgresRepository) RecalculateCounts(ctx context.Context, campaignID uuid.UUID) error {
	query := `
		UPDATE campaigns c SET
			sent_count = (SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = c.id AND status IN ('sent', 'delivered', 'read')),
			delivered_count = (SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = c.id AND status IN ('delivered', 'read')),
			read_count = (SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = c.id AND status = 'read'),
			failed_count = (SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = c.id AND status = 'failed'),
			updated_at = NOW()
		WHERE c.id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, campaignID)
	return err
}

func (r *PostgresRepository) scanCampaign(row pgx.Row) (*Campaign, error) {
	var c Campaign
	var status string
	var mapping []byte
	err := row.Scan(
		&c.ID, &c.TeamID, &c.DeviceID, &c.TemplateID, &c.Name, &status, &mapping,
		&c.ScheduledAt, &c.StartedAt, &c.CompletedAt, &c.CreatedBy,
		&c.TotalContacts, &c.SentCount, &c.DeliveredCount, &c.ReadCount, &c.FailedCount,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan campaign: %w", err)
	}
	c.Status = Status(status)
	c.VariableMapping = json.RawMessage(mapping)
	return &c, nil
}

func (r *PostgresRepository) scanCampaignRow(rows pgx.Rows) (*Campaign, error) {
	var c Campaign
	var status string
	var mapping []byte
	err := rows.Scan(
		&c.ID, &c.TeamID, &c.DeviceID, &c.TemplateID, &c.Name, &status, &mapping,
		&c.ScheduledAt, &c.StartedAt, &c.CompletedAt, &c.CreatedBy,
		&c.TotalContacts, &c.SentCount, &c.DeliveredCount, &c.ReadCount, &c.FailedCount,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan campaign row: %w", err)
	}
	c.Status = Status(status)
	c.VariableMapping = json.RawMessage(mapping)
	return &c, nil
}

func (r *PostgresRepository) scanContact(row pgx.Row) (*CampaignContact, error) {
	var cc CampaignContact
	var status string
	err := row.Scan(
		&cc.ID, &cc.CampaignID, &cc.ContactID, &cc.Phone, &status,
		&cc.Wamid, &cc.ErrorMessage, &cc.SentAt, &cc.DeliveredAt, &cc.ReadAt, &cc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("campaign contact not found")
		}
		return nil, fmt.Errorf("scan campaign contact: %w", err)
	}
	cc.Status = ContactStatus(status)
	return &cc, nil
}

func (r *PostgresRepository) scanContactRow(rows pgx.Rows) (*CampaignContact, error) {
	var cc CampaignContact
	var status string
	err := rows.Scan(
		&cc.ID, &cc.CampaignID, &cc.ContactID, &cc.Phone, &status,
		&cc.Wamid, &cc.ErrorMessage, &cc.SentAt, &cc.DeliveredAt, &cc.ReadAt, &cc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan campaign contact row: %w", err)
	}
	cc.Status = ContactStatus(status)
	return &cc, nil
}
