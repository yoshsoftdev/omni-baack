package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

const channelSelectColumns = `
	id, team_id, name, provider, channel_type, status, connection_type,
	phone_number, waba_id, phone_number_id, business_id, ig_user_id, is_coex, health_metadata, last_connected_at,
	created_at, updated_at, deleted_at
`

// Repository (ChannelRepository) persists channels and related records.
type Repository interface {
	Create(ctx context.Context, ch *Channel) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Channel, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Channel], error)
	Update(ctx context.Context, ch *Channel) error
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
	CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error)

	UpsertCredentials(ctx context.Context, cred *Credentials) error
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*Credentials, error)
	DeleteCredentials(ctx context.Context, channelID uuid.UUID) error

	UpsertOnboarding(ctx context.Context, ob *Onboarding) error
	FindOnboarding(ctx context.Context, channelID uuid.UUID) (*Onboarding, error)
	CountMetaReady(ctx context.Context, teamID uuid.UUID) (int64, error)
	FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Channel, error)
	FindByWABAID(ctx context.Context, wabaID string) (*Channel, error)
	FindByIGUserID(ctx context.Context, teamID uuid.UUID, igUserID string) (*Channel, error)
	FindByIGUserIDGlobal(ctx context.Context, igUserID string) (*Channel, error)
	FindConnectedByType(ctx context.Context, teamID uuid.UUID, channelType ChannelType) (*Channel, error)
	UpdateHealthMetadata(ctx context.Context, channelID uuid.UUID, metadata []byte) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, ch *Channel) error {
	if ch.ChannelType == "" {
		ch.ChannelType = TypeWhatsApp
	}
	query := `
		INSERT INTO channels (
			id, team_id, name, provider, channel_type, status, connection_type,
			phone_number, waba_id, phone_number_id, business_id, ig_user_id, is_coex, health_metadata, last_connected_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`
	health := ch.HealthMetadata
	if len(health) == 0 {
		health = []byte("{}")
	}
	_, err := r.db.Pool.Exec(ctx, query,
		ch.ID, ch.TeamID, ch.Name, ch.Provider, string(ch.ChannelType), string(ch.Status), ch.ConnectionType,
		ch.PhoneNumber, ch.WABAID, ch.PhoneNumberID, ch.BusinessID, ch.IGUserID, ch.IsCoex, health, ch.LastConnectedAt,
		ch.CreatedAt, ch.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Channel, error) {
	query := `SELECT ` + channelSelectColumns + ` FROM channels WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, id, teamID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Channel], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.ChannelType != nil {
		args = append(args, string(*filter.ChannelType))
		where += fmt.Sprintf(` AND channel_type = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}

	var total int64
	countQ := `SELECT COUNT(*) FROM channels WHERE ` + where
	if err := r.db.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return repository.Page[Channel]{}, fmt.Errorf("count channels: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	listQ := fmt.Sprintf(`
		SELECT %s
		FROM channels WHERE %s
		ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, channelSelectColumns, where, len(args)-1, len(args))

	rows, err := r.db.Pool.Query(ctx, listQ, args...)
	if err != nil {
		return repository.Page[Channel]{}, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	items := make([]Channel, 0)
	for rows.Next() {
		ch, err := scanChannelRow(rows)
		if err != nil {
			return repository.Page[Channel]{}, err
		}
		items = append(items, *ch)
	}
	return repository.Page[Channel]{
		Items:      items,
		Pagination: repository.NewMeta(page.Page, page.PageSize, total),
	}, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, ch *Channel) error {
	query := `
		UPDATE channels SET
			name = $1, status = $2, connection_type = $3, phone_number = $4,
			waba_id = $5, phone_number_id = $6, business_id = $7, ig_user_id = $8, is_coex = $9, last_connected_at = $10,
			channel_type = $11, provider = $12, updated_at = $13
		WHERE id = $14 AND team_id = $15 AND deleted_at IS NULL
	`
	channelType := ch.ChannelType
	if channelType == "" {
		channelType = TypeWhatsApp
	}
	tag, err := r.db.Pool.Exec(ctx, query,
		ch.Name, string(ch.Status), ch.ConnectionType, ch.PhoneNumber,
		ch.WABAID, ch.PhoneNumberID, ch.BusinessID, ch.IGUserID, ch.IsCoex, ch.LastConnectedAt,
		string(channelType), ch.Provider, ch.UpdatedAt,
		ch.ID, ch.TeamID,
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
	query := `UPDATE channels SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Pool.Exec(ctx, query, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM channels WHERE team_id = $1 AND deleted_at IS NULL`, teamID,
	).Scan(&count)
	if err != nil {
		return 0, repository.MapError(err)
	}
	return count, nil
}

func (r *PostgresRepository) UpsertCredentials(ctx context.Context, cred *Credentials) error {
	query := `
		INSERT INTO channel_credentials (channel_id, access_token_enc, token_expires_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (channel_id) DO UPDATE SET
			access_token_enc = EXCLUDED.access_token_enc,
			token_expires_at = EXCLUDED.token_expires_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query, cred.ChannelID, cred.AccessTokenEnc, cred.TokenExpiresAt, cred.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindCredentials(ctx context.Context, channelID uuid.UUID) (*Credentials, error) {
	query := `SELECT channel_id, access_token_enc, token_expires_at, updated_at FROM channel_credentials WHERE channel_id = $1`
	var c Credentials
	err := r.db.Pool.QueryRow(ctx, query, channelID).Scan(&c.ChannelID, &c.AccessTokenEnc, &c.TokenExpiresAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find credentials: %w", err)
	}
	return &c, nil
}

func (r *PostgresRepository) DeleteCredentials(ctx context.Context, channelID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM channel_credentials WHERE channel_id = $1`, channelID)
	return repository.MapError(err)
}

func (r *PostgresRepository) UpsertOnboarding(ctx context.Context, ob *Onboarding) error {
	query := `
		INSERT INTO channel_onboarding (
			channel_id, status, contacts_synced_count, history_messages_synced,
			history_sync_progress, error_message, completed_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (channel_id) DO UPDATE SET
			status = EXCLUDED.status,
			contacts_synced_count = EXCLUDED.contacts_synced_count,
			history_messages_synced = EXCLUDED.history_messages_synced,
			history_sync_progress = EXCLUDED.history_sync_progress,
			error_message = EXCLUDED.error_message,
			completed_at = EXCLUDED.completed_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query,
		ob.ChannelID, string(ob.Status), ob.ContactsSyncedCount, ob.HistoryMessagesSynced,
		ob.HistorySyncProgress, ob.ErrorMessage, ob.CompletedAt, ob.UpdatedAt,
	)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindOnboarding(ctx context.Context, channelID uuid.UUID) (*Onboarding, error) {
	query := `
		SELECT channel_id, status, contacts_synced_count, history_messages_synced,
			history_sync_progress, error_message, completed_at, updated_at
		FROM channel_onboarding WHERE channel_id = $1
	`
	var ob Onboarding
	var status string
	err := r.db.Pool.QueryRow(ctx, query, channelID).Scan(
		&ob.ChannelID, &status, &ob.ContactsSyncedCount, &ob.HistoryMessagesSynced,
		&ob.HistorySyncProgress, &ob.ErrorMessage, &ob.CompletedAt, &ob.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find onboarding: %w", err)
	}
	ob.Status = OnboardingStatus(status)
	return &ob, nil
}

func (r *PostgresRepository) CountMetaReady(ctx context.Context, teamID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM channels c
		INNER JOIN channel_credentials cc ON cc.channel_id = c.id
		INNER JOIN channel_onboarding co ON co.channel_id = c.id
		WHERE c.team_id = $1
			AND c.deleted_at IS NULL
			AND c.status = 'connected'
			AND co.status = 'completed'
			AND c.waba_id IS NOT NULL
			AND c.phone_number_id IS NOT NULL
			AND (cc.token_expires_at IS NULL OR cc.token_expires_at > NOW())
	`, teamID).Scan(&count)
	if err != nil {
		return 0, repository.MapError(err)
	}
	return count, nil
}

func (r *PostgresRepository) FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Channel, error) {
	query := `
		SELECT ` + channelSelectColumns + `
		FROM channels
		WHERE phone_number_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1
	`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, phoneNumberID))
}

func (r *PostgresRepository) FindByWABAID(ctx context.Context, wabaID string) (*Channel, error) {
	query := `
		SELECT ` + channelSelectColumns + `
		FROM channels
		WHERE waba_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1
	`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, wabaID))
}

func (r *PostgresRepository) FindByIGUserID(ctx context.Context, teamID uuid.UUID, igUserID string) (*Channel, error) {
	query := `
		SELECT ` + channelSelectColumns + `
		FROM channels
		WHERE team_id = $1 AND ig_user_id = $2 AND deleted_at IS NULL
		LIMIT 1
	`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, teamID, igUserID))
}

func (r *PostgresRepository) FindByIGUserIDGlobal(ctx context.Context, igUserID string) (*Channel, error) {
	igUserID = strings.TrimSpace(igUserID)
	if igUserID == "" {
		return nil, repository.ErrNotFound
	}
	query := `
		SELECT ` + channelSelectColumns + `
		FROM channels
		WHERE ig_user_id = $1 AND deleted_at IS NULL AND status = 'connected'
		ORDER BY updated_at DESC
		LIMIT 1
	`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, igUserID))
}

func (r *PostgresRepository) FindConnectedByType(ctx context.Context, teamID uuid.UUID, channelType ChannelType) (*Channel, error) {
	query := `
		SELECT ` + channelSelectColumns + `
		FROM channels
		WHERE team_id = $1 AND channel_type = $2 AND status = 'connected' AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1
	`
	return scanChannel(r.db.Pool.QueryRow(ctx, query, teamID, string(channelType)))
}

func (r *PostgresRepository) UpdateHealthMetadata(ctx context.Context, channelID uuid.UUID, metadata []byte) error {
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE channels SET health_metadata = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, metadata, channelID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanChannel(row pgx.Row) (*Channel, error) {
	ch, err := scanChannelRow(row)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func scanChannelRow(row pgx.Row) (*Channel, error) {
	var ch Channel
	var status string
	var channelType string
	var connType *string
	err := row.Scan(
		&ch.ID, &ch.TeamID, &ch.Name, &ch.Provider, &channelType, &status, &connType,
		&ch.PhoneNumber, &ch.WABAID, &ch.PhoneNumberID, &ch.BusinessID, &ch.IGUserID, &ch.IsCoex, &ch.HealthMetadata, &ch.LastConnectedAt,
		&ch.CreatedAt, &ch.UpdatedAt, &ch.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan channel: %w", err)
	}
	ch.Status = Status(status)
	if channelType == "" {
		channelType = string(TypeWhatsApp)
	}
	ch.ChannelType = ChannelType(channelType)
	if connType != nil {
		ct := ConnectionType(*connType)
		ch.ConnectionType = &ct
	}
	return &ch, nil
}
