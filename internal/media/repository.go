package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// AssetRepository persists MediaAsset metadata (id = storage_object_id).
type AssetRepository interface {
	Upsert(ctx context.Context, asset MediaAsset) error
	FindByID(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error)
}

// PostgresAssetRepository stores media_assets in PostgreSQL.
type PostgresAssetRepository struct {
	db *database.DB
}

func NewPostgresAssetRepository(db *database.DB) *PostgresAssetRepository {
	return &PostgresAssetRepository{db: db}
}

func (r *PostgresAssetRepository) Upsert(ctx context.Context, asset MediaAsset) error {
	if err := ValidateAsset(asset); err != nil {
		return err
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO media_assets (
			id, team_id, mime_type, file_size, storage_path, content_hash, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			team_id = EXCLUDED.team_id,
			mime_type = EXCLUDED.mime_type,
			file_size = EXCLUDED.file_size,
			storage_path = EXCLUDED.storage_path,
			content_hash = EXCLUDED.content_hash,
			updated_at = EXCLUDED.updated_at
	`, asset.ID, asset.TeamID, asset.MimeType, asset.FileSize, asset.StoragePath, asset.ContentHash,
		asset.CreatedAt, asset.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresAssetRepository) FindByID(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error) {
	row := r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, mime_type, file_size, storage_path, content_hash, created_at, updated_at
		FROM media_assets
		WHERE id = $1 AND team_id = $2
	`, assetID, teamID)
	asset, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return asset, nil
}

func scanAsset(row pgx.Row) (*MediaAsset, error) {
	var a MediaAsset
	err := row.Scan(
		&a.ID, &a.TeamID, &a.MimeType, &a.FileSize, &a.StoragePath, &a.ContentHash, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan media asset: %w", err)
	}
	return &a, nil
}

// ValidateAsset enforces MediaAsset.ID == storage_object_id rules.
func ValidateAsset(asset MediaAsset) error {
	if asset.ID == uuid.Nil {
		return ErrInvalidAssetID
	}
	if asset.TeamID == uuid.Nil {
		return fmt.Errorf("media asset team_id is required")
	}
	if asset.FileSize < 0 {
		return fmt.Errorf("media asset file_size must be non-negative")
	}
	return nil
}

// MemoryAssetRepository is an in-memory AssetRepository for unit tests.
type MemoryAssetRepository struct {
	byKey map[string]MediaAsset
}

func NewMemoryAssetRepository() *MemoryAssetRepository {
	return &MemoryAssetRepository{byKey: map[string]MediaAsset{}}
}

func memoryKey(teamID, assetID uuid.UUID) string {
	return teamID.String() + ":" + assetID.String()
}

func (r *MemoryAssetRepository) Upsert(_ context.Context, asset MediaAsset) error {
	if err := ValidateAsset(asset); err != nil {
		return err
	}
	cp := asset
	r.byKey[memoryKey(asset.TeamID, asset.ID)] = cp
	return nil
}

func (r *MemoryAssetRepository) FindByID(_ context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error) {
	asset, ok := r.byKey[memoryKey(teamID, assetID)]
	if !ok {
		return nil, ErrAssetNotFound
	}
	cp := asset
	return &cp, nil
}

// AssetRegistry registers and loads MediaAsset metadata.
// It implements the asset portion of Manager; EnsureMedia is provided by channel-specific managers.
type AssetRegistry struct {
	assets AssetRepository
}

func NewAssetRegistry(assets AssetRepository) *AssetRegistry {
	return &AssetRegistry{assets: assets}
}

func (r *AssetRegistry) RegisterAsset(ctx context.Context, asset MediaAsset) error {
	if err := ValidateAsset(asset); err != nil {
		return err
	}
	now := time.Now().UTC()
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	asset.UpdatedAt = now
	asset.MimeType = strings.TrimSpace(asset.MimeType)
	asset.StoragePath = strings.TrimSpace(asset.StoragePath)
	asset.ContentHash = strings.TrimSpace(asset.ContentHash)
	return r.assets.Upsert(ctx, asset)
}

func (r *AssetRegistry) FindAsset(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error) {
	if assetID == uuid.Nil {
		return nil, ErrInvalidAssetID
	}
	return r.assets.FindByID(ctx, teamID, assetID)
}
