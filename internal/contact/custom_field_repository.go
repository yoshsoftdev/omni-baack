package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func (r *PostgresRepository) SyncContactTags(ctx context.Context, teamID, contactID uuid.UUID, tagIDs []uuid.UUID) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM contacts WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
			)
		`, contactID, teamID).Scan(&exists); err != nil {
			return repository.MapError(err)
		}
		if !exists {
			return repository.ErrNotFound
		}

		desired := make(map[uuid.UUID]struct{}, len(tagIDs))
		for _, id := range tagIDs {
			desired[id] = struct{}{}
		}

		rows, err := tx.Query(ctx, `
			SELECT ct.tag_id
			FROM contact_tags ct
			INNER JOIN tags t ON t.id = ct.tag_id AND t.team_id = $1 AND t.deleted_at IS NULL
			WHERE ct.contact_id = $2
		`, teamID, contactID)
		if err != nil {
			return repository.MapError(err)
		}
		current := make(map[uuid.UUID]struct{})
		for rows.Next() {
			var tagID uuid.UUID
			if err := rows.Scan(&tagID); err != nil {
				rows.Close()
				return err
			}
			current[tagID] = struct{}{}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for tagID := range desired {
			if _, ok := current[tagID]; ok {
				continue
			}
			var valid bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM tags WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
				)
			`, tagID, teamID).Scan(&valid); err != nil {
				return repository.MapError(err)
			}
			if !valid {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO contact_tags (contact_id, tag_id, created_at)
				VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING
			`, contactID, tagID); err != nil {
				return repository.MapError(err)
			}
		}

		for tagID := range current {
			if _, ok := desired[tagID]; ok {
				continue
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM contact_tags WHERE contact_id = $1 AND tag_id = $2
			`, contactID, tagID); err != nil {
				return repository.MapError(err)
			}
		}

		_, err = tx.Exec(ctx, `
			UPDATE tags t SET contact_count = (
				SELECT COUNT(*)::int
				FROM contact_tags ct
				INNER JOIN contacts c ON c.id = ct.contact_id AND c.deleted_at IS NULL
				WHERE ct.tag_id = t.id
			), updated_at = NOW()
			WHERE t.team_id = $1 AND t.deleted_at IS NULL
		`, teamID)
		return repository.MapError(err)
	})
}

func (r *PostgresRepository) CountMarketingByTeam(ctx context.Context, teamID uuid.UUID) (map[MarketingStatus]int64, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT marketing_status, COUNT(*)::bigint
		FROM contacts
		WHERE team_id = $1 AND deleted_at IS NULL
		GROUP BY marketing_status
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[MarketingStatus]int64{
		MarketingSubscribed:   0,
		MarketingUnsubscribed: 0,
		MarketingUnknown:      0,
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[MarketingStatus(status)] = count
	}
	return out, rows.Err()
}

func (r *PostgresRepository) LoadContactStats(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]ContactStats, error) {
	out := make(map[uuid.UUID]ContactStats)
	if len(contactIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT contact_id, COUNT(*)::int
		FROM conversations
		WHERE team_id = $1 AND contact_id = ANY($2) AND deleted_at IS NULL
		GROUP BY contact_id
	`, teamID, contactIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var contactID uuid.UUID
		var count int
		if err := rows.Scan(&contactID, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats := out[contactID]
		stats.ConversationsCount = count
		out[contactID] = stats
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	campaignRows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT ON (cr.contact_id)
			cr.contact_id, c.id, c.name, cr.created_at
		FROM campaign_recipients cr
		INNER JOIN campaigns c ON c.id = cr.campaign_id AND c.deleted_at IS NULL
		WHERE cr.team_id = $1 AND cr.contact_id = ANY($2)
		ORDER BY cr.contact_id, cr.created_at DESC
	`, teamID, contactIDs)
	if err != nil {
		return nil, err
	}
	defer campaignRows.Close()

	for campaignRows.Next() {
		var contactID, campaignID uuid.UUID
		var name string
		var sentAt time.Time
		if err := campaignRows.Scan(&contactID, &campaignID, &name, &sentAt); err != nil {
			return nil, err
		}
		stats := out[contactID]
		stats.LastCampaign = &LastCampaignRef{ID: campaignID, Name: name, SentAt: sentAt}
		out[contactID] = stats
	}
	return out, campaignRows.Err()
}

func (r *PostgresRepository) ListFieldDefinitions(ctx context.Context, teamID uuid.UUID) ([]FieldDefinition, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, team_id, field_key, label, field_type, options, sort_order, created_at, updated_at
		FROM contact_field_definitions
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY sort_order ASC, label ASC
	`, teamID)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	items := make([]FieldDefinition, 0)
	for rows.Next() {
		def, err := scanFieldDefinition(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *def)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) FindFieldDefinitionByID(ctx context.Context, teamID, id uuid.UUID) (*FieldDefinition, error) {
	return scanFieldDefinition(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, field_key, label, field_type, options, sort_order, created_at, updated_at
		FROM contact_field_definitions
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) CreateFieldDefinition(ctx context.Context, def *FieldDefinition) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO contact_field_definitions (
			id, team_id, field_key, label, field_type, options, sort_order, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, def.ID, def.TeamID, def.FieldKey, def.Label, string(def.FieldType), def.Options, def.SortOrder, def.CreatedAt, def.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) UpdateFieldDefinition(ctx context.Context, def *FieldDefinition) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE contact_field_definitions SET
			label = $1, field_type = $2, options = $3, sort_order = $4, updated_at = $5
		WHERE id = $6 AND team_id = $7 AND deleted_at IS NULL
	`, def.Label, string(def.FieldType), def.Options, def.SortOrder, def.UpdatedAt, def.ID, def.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteFieldDefinition(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE contact_field_definitions SET deleted_at = NOW(), updated_at = NOW()
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

func (r *PostgresRepository) ListCustomFieldsByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]CustomFieldValue, error) {
	out := make(map[uuid.UUID][]CustomFieldValue)
	if len(contactIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT v.contact_id, d.id, d.field_key, d.label, d.field_type, d.options, v.value
		FROM contact_field_values v
		INNER JOIN contact_field_definitions d ON d.id = v.field_definition_id AND d.team_id = $1 AND d.deleted_at IS NULL
		INNER JOIN contacts c ON c.id = v.contact_id AND c.team_id = $1 AND c.deleted_at IS NULL
		WHERE v.contact_id = ANY($2)
		ORDER BY d.sort_order ASC, d.label ASC
	`, teamID, contactIDs)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var contactID uuid.UUID
		var field CustomFieldValue
		var fieldType string
		var options []byte
		var value []byte
		if err := rows.Scan(&contactID, &field.ID, &field.FieldKey, &field.Label, &fieldType, &options, &value); err != nil {
			return nil, err
		}
		field.FieldType = FieldType(fieldType)
		if len(options) > 0 {
			field.Options = json.RawMessage(options)
		}
		field.Value = json.RawMessage(value)
		out[contactID] = append(out[contactID], field)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertCustomFieldValues(ctx context.Context, contactID uuid.UUID, values map[uuid.UUID]json.RawMessage) error {
	if len(values) == 0 {
		return nil
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		for fieldID, value := range values {
			if _, err := tx.Exec(ctx, `
				INSERT INTO contact_field_values (contact_id, field_definition_id, value, updated_at)
				VALUES ($1, $2, $3, NOW())
				ON CONFLICT (contact_id, field_definition_id)
				DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
			`, contactID, fieldID, value); err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func scanFieldDefinition(row pgx.Row) (*FieldDefinition, error) {
	var def FieldDefinition
	var fieldType string
	var options []byte
	err := row.Scan(&def.ID, &def.TeamID, &def.FieldKey, &def.Label, &fieldType, &options, &def.SortOrder, &def.CreatedAt, &def.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan field definition: %w", err)
	}
	def.FieldType = FieldType(fieldType)
	if len(options) > 0 {
		def.Options = json.RawMessage(options)
	}
	return &def, nil
}

func normalizeFieldKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, " ", "_")
	return key
}
