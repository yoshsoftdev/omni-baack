package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type LinkResult struct {
	Profile           *Profile
	PreviousContactID uuid.UUID
}

type UnlinkResult struct {
	Profile      *Profile
	NewContactID uuid.UUID
}

// Repository persists canonical provider identities and their ownership audit.
type Repository interface {
	Create(ctx context.Context, profile *Profile) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Profile, error)
	FindByExternalID(ctx context.Context, teamID uuid.UUID, kind Kind, providerUserID string) (*Profile, error)
	UpdateObserved(ctx context.Context, profile *Profile) error
	List(ctx context.Context, teamID uuid.UUID) ([]Profile, error)
	ListByContact(ctx context.Context, teamID, contactID uuid.UUID) ([]Profile, error)
	ListByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]Profile, error)
	Link(ctx context.Context, teamID, profileID, targetContactID, actorID uuid.UUID, reason string) (*LinkResult, error)
	Unlink(ctx context.Context, teamID, profileID, actorID uuid.UUID, reason string) (*UnlinkResult, error)
	SetSourceOfTruth(ctx context.Context, teamID, profileID, actorID uuid.UUID) (*Profile, error)
	ListActivities(ctx context.Context, teamID, contactID uuid.UUID, limit int) ([]Activity, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, p *Profile) error {
	if len(p.Metadata) == 0 {
		p.Metadata = json.RawMessage(`{}`)
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO provider_profiles (
			id, team_id, contact_id, provider, provider_user_id, channel_id,
			display_name, username, phone, avatar_url, email, metadata,
			connected_at, last_synced_at, last_observed_at, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, p.ID, p.TeamID, p.ContactID, string(p.Provider), p.ProviderUserID, p.ChannelID,
		nullable(p.DisplayName), nullable(p.Username), nullable(p.Phone), nullable(p.AvatarURL), nullable(p.Email), p.Metadata,
		p.ConnectedAt, p.LastSyncedAt, p.LastObservedAt, string(p.Status), p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return repository.MapError(err)
	}
	if p.Provider == WhatsApp {
		_, err = r.db.Pool.Exec(ctx, `
			UPDATE contacts
			SET source_of_truth_profile_id = COALESCE(source_of_truth_profile_id, $1), updated_at = NOW()
			WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
		`, p.ID, p.ContactID, p.TeamID)
	}
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Profile, error) {
	return scanProfile(r.db.Pool.QueryRow(ctx, profileSelect+`
		WHERE p.team_id = $1 AND p.id = $2 AND p.deleted_at IS NULL
		GROUP BY p.id, c.source_of_truth_profile_id
	`, teamID, id))
}

func (r *PostgresRepository) FindByExternalID(ctx context.Context, teamID uuid.UUID, kind Kind, providerUserID string) (*Profile, error) {
	return scanProfile(r.db.Pool.QueryRow(ctx, profileSelect+`
		WHERE p.team_id = $1 AND p.provider = $2 AND p.provider_user_id = $3 AND p.deleted_at IS NULL
		GROUP BY p.id, c.source_of_truth_profile_id
		ORDER BY p.last_observed_at DESC NULLS LAST, p.connected_at ASC
		LIMIT 1
	`, teamID, string(kind), strings.TrimSpace(providerUserID)))
}

func (r *PostgresRepository) UpdateObserved(ctx context.Context, p *Profile) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE provider_profiles SET
			display_name = NULLIF($1, ''),
			username = NULLIF($2, ''),
			phone = NULLIF($3, ''),
			avatar_url = NULLIF($4, ''),
			email = NULLIF($5, ''),
			metadata = COALESCE($6, metadata),
			last_observed_at = $7,
			status = $8,
			updated_at = NOW()
		WHERE id = $9 AND team_id = $10 AND deleted_at IS NULL
	`, strings.TrimSpace(p.DisplayName), strings.TrimSpace(p.Username), strings.TrimSpace(p.Phone),
		strings.TrimSpace(p.AvatarURL), strings.TrimSpace(p.Email), p.Metadata, p.LastObservedAt,
		string(p.Status), p.ID, p.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID) ([]Profile, error) {
	return r.list(ctx, profileSelect+`
		WHERE p.team_id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id, c.source_of_truth_profile_id
		ORDER BY p.connected_at DESC
	`, teamID)
}

func (r *PostgresRepository) ListByContact(ctx context.Context, teamID, contactID uuid.UUID) ([]Profile, error) {
	return r.list(ctx, profileSelect+`
		WHERE p.team_id = $1 AND p.contact_id = $2 AND p.deleted_at IS NULL
		GROUP BY p.id, c.source_of_truth_profile_id
		ORDER BY p.connected_at ASC
	`, teamID, contactID)
}

func (r *PostgresRepository) ListByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]Profile, error) {
	out := make(map[uuid.UUID][]Profile)
	if len(contactIDs) == 0 {
		return out, nil
	}
	items, err := r.list(ctx, profileSelect+`
		WHERE p.team_id = $1 AND p.contact_id = ANY($2) AND p.deleted_at IS NULL
		GROUP BY p.id, c.source_of_truth_profile_id
		ORDER BY p.connected_at ASC
	`, teamID, contactIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		out[item.ContactID] = append(out[item.ContactID], item)
	}
	return out, nil
}

func (r *PostgresRepository) Link(ctx context.Context, teamID, profileID, targetContactID, actorID uuid.UUID, reason string) (*LinkResult, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := findProfileForUpdate(ctx, tx, teamID, profileID)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM contacts WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL)
	`, targetContactID, teamID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, repository.ErrNotFound
	}
	previous := profile.ContactID
	if previous == targetContactID {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &LinkResult{Profile: profile, PreviousContactID: previous}, nil
	}

	var wasSource bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(source_of_truth_profile_id = $1, false)
		FROM contacts
		WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
		FOR UPDATE
	`, profileID, previous, teamID).Scan(&wasSource); err != nil {
		return nil, repository.MapError(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contacts SET
			source_of_truth_profile_id = NULL,
			phone = CASE WHEN $4 AND phone = $5 THEN NULL ELSE phone END,
			updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND source_of_truth_profile_id = $3
	`, previous, teamID, profileID, wasSource && profile.Provider == WhatsApp, nullable(profile.Phone)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_profiles SET contact_id = $1, status = 'active', updated_at = NOW()
		WHERE id = $2 AND team_id = $3
	`, targetContactID, profileID, teamID); err != nil {
		return nil, repository.MapError(err)
	}
	if profile.Provider == WhatsApp {
		if _, err := tx.Exec(ctx, `
			UPDATE contacts SET
				phone = CASE WHEN source_of_truth_profile_id IS NULL THEN NULLIF($4, '') ELSE phone END,
				source_of_truth_profile_id = COALESCE(source_of_truth_profile_id, $1),
				updated_at = NOW()
			WHERE id = $2 AND team_id = $3
		`, profileID, targetContactID, teamID, profile.Phone); err != nil {
			return nil, err
		}
	}
	meta, _ := json.Marshal(map[string]any{
		"previous_contact_id": previous,
		"new_contact_id":      targetContactID,
		"provider":            profile.Provider,
		"reason":              strings.TrimSpace(reason),
	})
	if err := insertActivity(ctx, tx, teamID, targetContactID, profileID, previous, targetContactID, ActivityLinked, actorID,
		fmt.Sprintf("Linked %s account", providerLabel(profile.Provider)), meta); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	profile.ContactID = targetContactID
	profile.Status = StatusActive
	profile.UpdatedAt = time.Now().UTC()
	return &LinkResult{Profile: profile, PreviousContactID: previous}, nil
}

func (r *PostgresRepository) Unlink(ctx context.Context, teamID, profileID, actorID uuid.UUID, reason string) (*UnlinkResult, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := findProfileForUpdate(ctx, tx, teamID, profileID)
	if err != nil {
		return nil, err
	}
	oldContactID := profile.ContactID
	newContactID := uuid.New()
	name := strings.TrimSpace(profile.DisplayName)
	if name == "" {
		name = strings.TrimSpace(profile.Username)
	}
	if name == "" {
		name = providerLabel(profile.Provider) + " User"
	}
	source := "manual"
	if profile.Provider == WhatsApp {
		source = "whatsapp"
	} else if profile.Provider == Instagram {
		source = "ig"
	}

	var wasSource bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(source_of_truth_profile_id = $1, false)
		FROM contacts WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
	`, profileID, oldContactID, teamID).Scan(&wasSource); err != nil {
		return nil, repository.MapError(err)
	}
	var canonicalPhone any
	if profile.Provider == WhatsApp && wasSource && strings.TrimSpace(profile.Phone) != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE contacts SET phone = NULL, source_of_truth_profile_id = NULL, updated_at = NOW()
			WHERE id = $1 AND team_id = $2
		`, oldContactID, teamID); err != nil {
			return nil, err
		}
		canonicalPhone = profile.Phone
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO contacts (
			id, team_id, contact_name, customer_name, phone, email, avatar_url,
			source, marketing_status, created_at, updated_at
		) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,'unknown',NOW(),NOW())
	`, newContactID, teamID, name, canonicalPhone, nullable(profile.Email), nullable(profile.AvatarURL), source); err != nil {
		return nil, repository.MapError(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_profiles SET contact_id = $1, status = 'active', updated_at = NOW()
		WHERE id = $2 AND team_id = $3
	`, newContactID, profileID, teamID); err != nil {
		return nil, repository.MapError(err)
	}
	if profile.Provider == WhatsApp {
		if _, err := tx.Exec(ctx, `
			UPDATE contacts SET source_of_truth_profile_id = $1 WHERE id = $2 AND team_id = $3
		`, profileID, newContactID, teamID); err != nil {
			return nil, err
		}
	}
	meta, _ := json.Marshal(map[string]any{
		"previous_contact_id": oldContactID,
		"new_contact_id":      newContactID,
		"provider":            profile.Provider,
		"reason":              strings.TrimSpace(reason),
	})
	if err := insertActivity(ctx, tx, teamID, oldContactID, profileID, oldContactID, newContactID, ActivityUnlinked, actorID,
		fmt.Sprintf("Unlinked %s account", providerLabel(profile.Provider)), meta); err != nil {
		return nil, err
	}
	if err := insertActivity(ctx, tx, teamID, newContactID, profileID, oldContactID, newContactID, ActivityUnlinked, actorID,
		fmt.Sprintf("%s account became a standalone contact", providerLabel(profile.Provider)), meta); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	profile.ContactID = newContactID
	profile.Status = StatusActive
	return &UnlinkResult{Profile: profile, NewContactID: newContactID}, nil
}

func (r *PostgresRepository) SetSourceOfTruth(ctx context.Context, teamID, profileID, actorID uuid.UUID) (*Profile, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	profile, err := findProfileForUpdate(ctx, tx, teamID, profileID)
	if err != nil {
		return nil, err
	}
	var previous *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT source_of_truth_profile_id FROM contacts
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, profile.ContactID, teamID).Scan(&previous); err != nil {
		return nil, repository.MapError(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contacts SET source_of_truth_profile_id = $1, phone = NULLIF($4, ''), updated_at = NOW()
		WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
	`, profileID, profile.ContactID, teamID, profile.Phone); err != nil {
		return nil, repository.MapError(err)
	}
	meta, _ := json.Marshal(map[string]any{
		"previous_profile_id": previous,
		"new_profile_id":      profileID,
		"provider":            profile.Provider,
	})
	if err := insertActivity(ctx, tx, teamID, profile.ContactID, profileID, profile.ContactID, profile.ContactID, ActivitySourceOfTruthChanged, actorID,
		"Source of truth changed to WhatsApp", meta); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	profile.IsSourceOfTruth = true
	return profile, nil
}

func (r *PostgresRepository) ListActivities(ctx context.Context, teamID, contactID uuid.UUID, limit int) ([]Activity, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT a.id, a.team_id, a.contact_id, a.provider_profile_id, a.activity_type,
		       a.actor_id, COALESCE(u.name, ''), a.description, a.metadata, a.created_at
		FROM contact_provider_activities a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.team_id = $1 AND a.contact_id = $2
		ORDER BY a.created_at DESC LIMIT $3
	`, teamID, contactID, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	items := make([]Activity, 0)
	for rows.Next() {
		var item Activity
		if err := rows.Scan(&item.ID, &item.TeamID, &item.ContactID, &item.ProviderProfileID, &item.Type,
			&item.ActorID, &item.ActorName, &item.Description, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, repository.MapError(err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) list(ctx context.Context, query string, args ...any) ([]Profile, error) {
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()
	items := make([]Profile, 0)
	for rows.Next() {
		item, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

const profileSelect = `
	SELECT p.id, p.team_id, p.contact_id, p.provider, p.provider_user_id, p.channel_id,
	       COALESCE(p.display_name, ''), COALESCE(p.username, ''), COALESCE(p.phone, ''),
	       COALESCE(p.avatar_url, ''), COALESCE(p.email, ''), p.metadata,
	       p.connected_at, p.last_synced_at, p.last_observed_at, p.status,
	       COALESCE(c.source_of_truth_profile_id = p.id, false),
	       COALESCE(array_agg(cv.id) FILTER (WHERE cv.id IS NOT NULL), '{}'),
	       p.created_at, p.updated_at
	FROM provider_profiles p
	JOIN contacts c ON c.id = p.contact_id AND c.team_id = p.team_id AND c.deleted_at IS NULL
	LEFT JOIN conversations cv ON cv.provider_profile_id = p.id AND cv.deleted_at IS NULL
`

func scanProfile(row pgx.Row) (*Profile, error) {
	var p Profile
	var providerValue, statusValue string
	err := row.Scan(&p.ID, &p.TeamID, &p.ContactID, &providerValue, &p.ProviderUserID, &p.ChannelID,
		&p.DisplayName, &p.Username, &p.Phone, &p.AvatarURL, &p.Email, &p.Metadata,
		&p.ConnectedAt, &p.LastSyncedAt, &p.LastObservedAt, &statusValue, &p.IsSourceOfTruth,
		&p.ConversationIDs, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan provider profile: %w", err)
	}
	p.Provider = Kind(providerValue)
	p.Status = Status(statusValue)
	return &p, nil
}

func findProfileForUpdate(ctx context.Context, tx pgx.Tx, teamID, profileID uuid.UUID) (*Profile, error) {
	var p Profile
	var providerValue, statusValue string
	err := tx.QueryRow(ctx, `
		SELECT id, team_id, contact_id, provider, provider_user_id, channel_id,
		       COALESCE(display_name, ''), COALESCE(username, ''), COALESCE(phone, ''),
		       COALESCE(avatar_url, ''), COALESCE(email, ''), metadata,
		       connected_at, last_synced_at, last_observed_at, status, created_at, updated_at
		FROM provider_profiles
		WHERE team_id = $1 AND id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, teamID, profileID).Scan(&p.ID, &p.TeamID, &p.ContactID, &providerValue, &p.ProviderUserID, &p.ChannelID,
		&p.DisplayName, &p.Username, &p.Phone, &p.AvatarURL, &p.Email, &p.Metadata,
		&p.ConnectedAt, &p.LastSyncedAt, &p.LastObservedAt, &statusValue, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, repository.MapError(err)
	}
	p.Provider = Kind(providerValue)
	p.Status = Status(statusValue)
	return &p, nil
}

func insertActivity(ctx context.Context, tx pgx.Tx, teamID, contactID, profileID, previousContactID, newContactID uuid.UUID, activityType ActivityType, actorID uuid.UUID, description string, metadata []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO contact_provider_activities (
			id, team_id, contact_id, provider_profile_id, previous_contact_id, new_contact_id,
			activity_type, actor_id, description, metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
	`, uuid.New(), teamID, contactID, profileID, previousContactID, newContactID,
		string(activityType), actorID, description, metadata)
	return repository.MapError(err)
}

func nullable(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func providerLabel(kind Kind) string {
	value := string(kind)
	if value == "" {
		return "Provider"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

var _ Repository = (*PostgresRepository)(nil)
