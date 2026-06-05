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

func (r *PostgresRepository) Create(ctx context.Context, device *Device) error {
	query := `
		INSERT INTO devices (
			id, team_id, name, phone_number, waba_id, phone_number_id, access_token,
			provider, status, last_connected_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		device.ID,
		device.TeamID,
		device.Name,
		device.PhoneNumber,
		device.WabaID,
		device.PhoneNumberID,
		device.AccessToken,
		device.Provider,
		string(device.Status),
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
	query := `
		SELECT id, team_id, name, phone_number, waba_id, phone_number_id, access_token,
		       provider, status, last_connected_at, created_at, updated_at, deleted_at
		FROM devices
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, deviceID, teamID))
}

func (r *PostgresRepository) FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Device, error) {
	query := `
		SELECT id, team_id, name, phone_number, waba_id, phone_number_id, access_token,
		       provider, status, last_connected_at, created_at, updated_at, deleted_at
		FROM devices
		WHERE phone_number_id = $1 AND deleted_at IS NULL
	`
	return r.scanOne(r.db.Pool.QueryRow(ctx, query, phoneNumberID))
}

func (r *PostgresRepository) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Device, error) {
	query := `
		SELECT id, team_id, name, phone_number, waba_id, phone_number_id, access_token,
		       provider, status, last_connected_at, created_at, updated_at, deleted_at
		FROM devices
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
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
		SET name = $1, phone_number = $2, waba_id = $3, phone_number_id = $4,
		    access_token = $5, provider = $6, status = $7, last_connected_at = $8, updated_at = $9
		WHERE id = $10 AND team_id = $11 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		device.Name,
		device.PhoneNumber,
		device.WabaID,
		device.PhoneNumberID,
		device.AccessToken,
		device.Provider,
		string(device.Status),
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

func (r *PostgresRepository) scanOne(row pgx.Row) (*Device, error) {
	var d Device
	var status string
	var deletedAt *time.Time
	err := row.Scan(
		&d.ID,
		&d.TeamID,
		&d.Name,
		&d.PhoneNumber,
		&d.WabaID,
		&d.PhoneNumberID,
		&d.AccessToken,
		&d.Provider,
		&status,
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
	d.DeletedAt = deletedAt
	return &d, nil
}

func (r *PostgresRepository) scanRow(row pgx.Rows) (*Device, error) {
	var d Device
	var status string
	var deletedAt *time.Time
	err := row.Scan(
		&d.ID,
		&d.TeamID,
		&d.Name,
		&d.PhoneNumber,
		&d.WabaID,
		&d.PhoneNumberID,
		&d.AccessToken,
		&d.Provider,
		&status,
		&d.LastConnectedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan device row: %w", err)
	}
	d.Status = Status(status)
	d.DeletedAt = deletedAt
	return &d, nil
}
