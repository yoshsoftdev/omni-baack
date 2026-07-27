package media

import (
	"context"

	"github.com/google/uuid"
)

// EnsureInput is the caller-supplied context for ensuring a provider media id.
// Credentials are always provided by the caller — Media Manager never loads them.
type EnsureInput struct {
	TeamID        uuid.UUID
	AssetID       uuid.UUID
	PhoneNumberID string
	AccessToken   string
	// ForceRefresh bypasses a still-valid cache entry and re-uploads.
	ForceRefresh bool
}

// Manager is the provider-agnostic Media Manager contract.
// WhatsApp-specific upload/cache lives behind an implementation (e.g. WhatsAppMediaManager).
type Manager interface {
	// EnsureMedia returns a usable MediaReference for the asset.
	// Primary path: storage-backed asset → provider media id (cached/refreshed).
	EnsureMedia(ctx context.Context, in EnsureInput) (*MediaReference, error)

	// RegisterAsset upserts MediaAsset metadata. Asset.ID must equal storage_object_id.
	RegisterAsset(ctx context.Context, asset MediaAsset) error

	// FindAsset loads MediaAsset metadata by storage_object_id.
	FindAsset(ctx context.Context, teamID, assetID uuid.UUID) (*MediaAsset, error)
}
