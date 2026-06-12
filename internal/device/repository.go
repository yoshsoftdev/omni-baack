package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("device not found")

type Repository interface {
	Create(ctx context.Context, device *Device) error
	FindByID(ctx context.Context, teamID, deviceID uuid.UUID) (*Device, error)
	FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Device, error)
	FindBySlug(ctx context.Context, slug string) (*Device, error)
	ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Device, error)
	Update(ctx context.Context, device *Device) error
	SoftDelete(ctx context.Context, teamID, deviceID uuid.UUID) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const deviceColumns = `
	id, team_id, slug, name, phone_number, waba_id, phone_number_id, access_token,
	provider, status, connection_type, is_coex, onboarding_status,
	contacts_synced_count, history_messages_synced, history_sync_progress,
	contacts_sync_request_id, history_sync_request_id, onboarding_error,
	onboarding_completed_at, last_connected_at, created_at, updated_at, deleted_at
`

func (r *PostgresRepository) Create(ctx context.Context, device *Device) error {
	query := `
		INSERT INTO devices (
			id, team_id, slug, name, phone_number, waba_id, phone_number_id, access_token,
			provider, status, connection_type, is_coex, onboarding_status,
			contacts_synced_count, history_messages_synced, history_sync_progress,
			contacts_sync_request_id, history_sync_request_id, onboarding_error,
			onboarding_completed_at, last_connected_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		device.ID,
		device.TeamID,
		device.Slug,
		device.Name,
		device.PhoneNumber,
		device.WabaID,
		device.PhoneNumberID,
		device.AccessToken,
		device.Provider,
		string(device.Status),
		string(device.ConnectionType),
		device.IsCoex,
		onboardingStatusValue(device.OnboardingStatus),
		device.ContactsSyncedCount,
		device.HistoryMessagesSynced,
		device.HistorySyncProgress,
		device.ContactsSyncRequestID,
		device.HistorySyncRequestID,
		device.OnboardingError,
		device.OnboardingCompletedAt,
		device.LastConnectedAt,
		device.CreatedAt,
		device.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert device: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, deviceID uuid.UUID) (*Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, deviceID, teamID))
}

func (r *PostgresRepository) FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE phone_number_id = $1 AND deleted_at IS NULL`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, phoneNumberID))
}

func (r *PostgresRepository) FindBySlug(ctx context.Context, slug string) (*Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE slug = $1 AND deleted_at IS NULL`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, strings.TrimSpace(slug)))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE team_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`
	rows, err := r.db.Pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		d, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (r *PostgresRepository) Update(ctx context.Context, device *Device) error {
	query := `
		UPDATE devices
		SET slug = $1, name = $2, phone_number = $3, waba_id = $4, phone_number_id = $5,
		    access_token = $6, provider = $7, status = $8, connection_type = $9,
		    is_coex = $10, onboarding_status = $11, contacts_synced_count = $12,
		    history_messages_synced = $13, history_sync_progress = $14,
		    contacts_sync_request_id = $15, history_sync_request_id = $16,
		    onboarding_error = $17, onboarding_completed_at = $18,
		    last_connected_at = $19, updated_at = $20
		WHERE id = $21 AND team_id = $22 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		device.Slug,
		device.Name,
		device.PhoneNumber,
		device.WabaID,
		device.PhoneNumberID,
		device.AccessToken,
		device.Provider,
		string(device.Status),
		string(device.ConnectionType),
		device.IsCoex,
		onboardingStatusValue(device.OnboardingStatus),
		device.ContactsSyncedCount,
		device.HistoryMessagesSynced,
		device.HistorySyncProgress,
		device.ContactsSyncRequestID,
		device.HistorySyncRequestID,
		device.OnboardingError,
		device.OnboardingCompletedAt,
		device.LastConnectedAt,
		device.UpdatedAt,
		device.ID,
		device.TeamID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("phone number id already registered")
		}
		return fmt.Errorf("update device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, deviceID uuid.UUID) error {
	query := `
		UPDATE devices
		SET deleted_at = NOW(), updated_at = NOW(), status = 'disconnected',
		    access_token = NULL, phone_number_id = NULL, waba_id = NULL
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, deviceID, teamID)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func onboardingStatusValue(status *OnboardingStatus) *string {
	if status == nil {
		return nil
	}
	value := string(*status)
	return &value
}

func (r *PostgresRepository) scanOne(row pgx.Row) (*Device, error) {
	var d Device
	var status, connectionType string
	var onboardingStatus *string
	var deletedAt *time.Time
	err := row.Scan(
		&d.ID,
		&d.TeamID,
		&d.Slug,
		&d.Name,
		&d.PhoneNumber,
		&d.WabaID,
		&d.PhoneNumberID,
		&d.AccessToken,
		&d.Provider,
		&status,
		&connectionType,
		&d.IsCoex,
		&onboardingStatus,
		&d.ContactsSyncedCount,
		&d.HistoryMessagesSynced,
		&d.HistorySyncProgress,
		&d.ContactsSyncRequestID,
		&d.HistorySyncRequestID,
		&d.OnboardingError,
		&d.OnboardingCompletedAt,
		&d.LastConnectedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan device: %w", err)
	}
	d.Status = Status(status)
	d.ConnectionType = ConnectionType(connectionType)
	if onboardingStatus != nil {
		value := OnboardingStatus(*onboardingStatus)
		d.OnboardingStatus = &value
	}
	d.DeletedAt = deletedAt
	return &d, nil
}

func (r *PostgresRepository) scanRow(row pgx.Rows) (*Device, error) {
	var d Device
	var status, connectionType string
	var onboardingStatus *string
	var deletedAt *time.Time
	err := row.Scan(
		&d.ID,
		&d.TeamID,
		&d.Slug,
		&d.Name,
		&d.PhoneNumber,
		&d.WabaID,
		&d.PhoneNumberID,
		&d.AccessToken,
		&d.Provider,
		&status,
		&connectionType,
		&d.IsCoex,
		&onboardingStatus,
		&d.ContactsSyncedCount,
		&d.HistoryMessagesSynced,
		&d.HistorySyncProgress,
		&d.ContactsSyncRequestID,
		&d.HistorySyncRequestID,
		&d.OnboardingError,
		&d.OnboardingCompletedAt,
		&d.LastConnectedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan device row: %w", err)
	}
	d.Status = Status(status)
	d.ConnectionType = ConnectionType(connectionType)
	if onboardingStatus != nil {
		value := OnboardingStatus(*onboardingStatus)
		d.OnboardingStatus = &value
	}
	d.DeletedAt = deletedAt
	return &d, nil
}
