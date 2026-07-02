package channelsync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type Repository interface {
	EnsureState(ctx context.Context, channelID uuid.UUID) (*State, error)
	FindState(ctx context.Context, channelID uuid.UUID) (*State, error)
	UpdateState(ctx context.Context, state *State) error
	EnqueueContactEvent(ctx context.Context, event *ContactEvent) error
	ListPendingContactEvents(ctx context.Context, channelID uuid.UUID, limit int) ([]ContactEvent, error)
	MarkContactEventsProcessed(ctx context.Context, ids []uuid.UUID) error
	TeamDataSynced(ctx context.Context, teamID uuid.UUID) (bool, error)
	TeamSyncSummary(ctx context.Context, teamID uuid.UUID) (*TeamSyncSummary, error)
}

type TeamSyncSummary struct {
	LastContactSyncAt      *time.Time
	LastTemplateSyncAt     *time.Time
	LastConversationSyncAt *time.Time
	PermissionStatus       PermissionState
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) EnsureState(ctx context.Context, channelID uuid.UUID) (*State, error) {
	state, err := r.FindState(ctx, channelID)
	if err == nil {
		return state, nil
	}
	if !repository.IsNotFound(err) {
		return nil, err
	}
	now := time.Now().UTC()
	perms, _ := json.Marshal(defaultPermissions())
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO channel_sync_state (channel_id, permissions, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (channel_id) DO NOTHING
	`, channelID, perms, now)
	if err != nil {
		return nil, fmt.Errorf("ensure channel sync state: %w", err)
	}
	return r.FindState(ctx, channelID)
}

func (r *PostgresRepository) FindState(ctx context.Context, channelID uuid.UUID) (*State, error) {
	row := r.db.Pool.QueryRow(ctx, `
		SELECT channel_id, permissions, contacts_status, contacts_created, contacts_updated,
			templates_status, templates_synced, conversations_status, conversations_imported,
			conversations_reason, contacts_sync_request_id, history_sync_request_id,
			last_sync_at, contacts_last_sync_at, templates_last_sync_at, conversations_last_sync_at,
			last_permission_check_at, updated_at
		FROM channel_sync_state
		WHERE channel_id = $1
	`, channelID)

	var state State
	var permsRaw []byte
	var convReason *string
	var contactsReq, historyReq *string
	if err := row.Scan(
		&state.ChannelID, &permsRaw, &state.ContactsStatus, &state.ContactsCreated, &state.ContactsUpdated,
		&state.TemplatesStatus, &state.TemplatesSynced, &state.ConversationsStatus, &state.ConversationsImported,
		&convReason, &contactsReq, &historyReq,
		&state.LastSyncAt, &state.ContactsLastSyncAt, &state.TemplatesLastSyncAt, &state.ConversationsLastSyncAt,
		&state.LastPermissionCheckAt, &state.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find channel sync state: %w", err)
	}
	state.Permissions = permissionsFromJSON(permsRaw)
	state.ConversationsReason = convReason
	state.ContactsSyncRequestID = contactsReq
	state.HistorySyncRequestID = historyReq
	return &state, nil
}

func (r *PostgresRepository) UpdateState(ctx context.Context, state *State) error {
	perms, err := json.Marshal(state.Permissions)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	state.UpdatedAt = now
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE channel_sync_state SET
			permissions = $2,
			contacts_status = $3,
			contacts_created = $4,
			contacts_updated = $5,
			templates_status = $6,
			templates_synced = $7,
			conversations_status = $8,
			conversations_imported = $9,
			conversations_reason = $10,
			contacts_sync_request_id = $11,
			history_sync_request_id = $12,
			last_sync_at = $13,
			contacts_last_sync_at = $14,
			templates_last_sync_at = $15,
			conversations_last_sync_at = $16,
			last_permission_check_at = $17,
			updated_at = $18
		WHERE channel_id = $1
	`, state.ChannelID, perms, state.ContactsStatus, state.ContactsCreated, state.ContactsUpdated,
		state.TemplatesStatus, state.TemplatesSynced, state.ConversationsStatus, state.ConversationsImported,
		state.ConversationsReason, state.ContactsSyncRequestID, state.HistorySyncRequestID,
		state.LastSyncAt, state.ContactsLastSyncAt, state.TemplatesLastSyncAt, state.ConversationsLastSyncAt,
		state.LastPermissionCheckAt, now)
	if err != nil {
		return fmt.Errorf("update channel sync state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) EnqueueContactEvent(ctx context.Context, event *ContactEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO channel_sync_contact_events (
			id, channel_id, team_id, action, phone_number, full_name, first_name, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, event.ID, event.ChannelID, event.TeamID, event.Action, event.PhoneNumber, event.FullName, event.FirstName, event.CreatedAt)
	return err
}

func (r *PostgresRepository) ListPendingContactEvents(ctx context.Context, channelID uuid.UUID, limit int) ([]ContactEvent, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, channel_id, team_id, action, phone_number, full_name, first_name, created_at
		FROM channel_sync_contact_events
		WHERE channel_id = $1 AND processed_at IS NULL
		ORDER BY created_at ASC
		LIMIT $2
	`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ContactEvent, 0)
	for rows.Next() {
		var event ContactEvent
		if err := rows.Scan(
			&event.ID, &event.ChannelID, &event.TeamID, &event.Action,
			&event.PhoneNumber, &event.FullName, &event.FirstName, &event.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) MarkContactEventsProcessed(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE channel_sync_contact_events
		SET processed_at = NOW()
		WHERE id = ANY($1)
	`, ids)
	return err
}

func (r *PostgresRepository) TeamDataSynced(ctx context.Context, teamID uuid.UUID) (bool, error) {
	var synced bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM channel_sync_state css
			INNER JOIN channels c ON c.id = css.channel_id
			WHERE c.team_id = $1
				AND c.deleted_at IS NULL
				AND c.status = 'connected'
				AND css.contacts_status = 'completed'
				AND css.templates_status = 'completed'
		)
	`, teamID).Scan(&synced)
	return synced, err
}

func (r *PostgresRepository) TeamSyncSummary(ctx context.Context, teamID uuid.UUID) (*TeamSyncSummary, error) {
	row := r.db.Pool.QueryRow(ctx, `
		SELECT
			MAX(css.contacts_last_sync_at),
			MAX(css.templates_last_sync_at),
			MAX(css.conversations_last_sync_at),
			BOOL_AND((css.permissions->>'contacts')::boolean),
			BOOL_AND((css.permissions->>'templates')::boolean),
			BOOL_AND((css.permissions->>'conversation_sync')::boolean)
		FROM channel_sync_state css
		INNER JOIN channels c ON c.id = css.channel_id
		WHERE c.team_id = $1 AND c.deleted_at IS NULL AND c.status = 'connected'
	`, teamID)

	var summary TeamSyncSummary
	var contactsGranted, templatesGranted, convGranted *bool
	if err := row.Scan(
		&summary.LastContactSyncAt,
		&summary.LastTemplateSyncAt,
		&summary.LastConversationSyncAt,
		&contactsGranted,
		&templatesGranted,
		&convGranted,
	); err != nil {
		if err == pgx.ErrNoRows {
			return &TeamSyncSummary{PermissionStatus: PermissionMissing}, nil
		}
		return nil, err
	}
	perms := Permissions{
		Contacts:         contactsGranted != nil && *contactsGranted,
		Templates:        templatesGranted != nil && *templatesGranted,
		ConversationSync: convGranted != nil && *convGranted,
	}
	summary.PermissionStatus = aggregatePermissionState(perms, false)
	return &summary, nil
}
