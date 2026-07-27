package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewMediaAssetFromStorageUsesStorageObjectID(t *testing.T) {
	storageID := uuid.New()
	teamID := uuid.New()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	asset := NewMediaAssetFromStorage(storageID, teamID, "image/jpeg", "team/a/obj.jpg", "abc", 1024, now)
	if asset.ID != storageID {
		t.Fatalf("ID must equal storage_object_id, got %s want %s", asset.ID, storageID)
	}
	if asset.TeamID != teamID || asset.MimeType != "image/jpeg" || asset.FileSize != 1024 {
		t.Fatalf("unexpected asset %#v", asset)
	}
	if asset.StoragePath != "team/a/obj.jpg" || asset.ContentHash != "abc" {
		t.Fatalf("unexpected path/hash %#v", asset)
	}
}

func TestValidateAssetRejectsNilID(t *testing.T) {
	err := ValidateAsset(MediaAsset{TeamID: uuid.New()})
	if !errors.Is(err, ErrInvalidAssetID) {
		t.Fatalf("expected ErrInvalidAssetID, got %v", err)
	}
}

func TestAssetRegistryRegisterAndFind(t *testing.T) {
	repo := NewMemoryAssetRepository()
	reg := NewAssetRegistry(repo)
	storageID := uuid.New()
	teamID := uuid.New()

	asset := NewMediaAssetFromStorage(storageID, teamID, "image/png", "path/x", "hash1", 50, time.Time{})
	if err := reg.RegisterAsset(context.Background(), asset); err != nil {
		t.Fatalf("RegisterAsset: %v", err)
	}

	got, err := reg.FindAsset(context.Background(), teamID, storageID)
	if err != nil {
		t.Fatalf("FindAsset: %v", err)
	}
	if got.ID != storageID || got.MimeType != "image/png" || got.FileSize != 50 {
		t.Fatalf("unexpected asset %#v", got)
	}
}

func TestAssetRegistryUpsertUpdatesMetadata(t *testing.T) {
	repo := NewMemoryAssetRepository()
	reg := NewAssetRegistry(repo)
	storageID := uuid.New()
	teamID := uuid.New()

	first := NewMediaAssetFromStorage(storageID, teamID, "image/jpeg", "a", "h1", 10, time.Now().UTC())
	if err := reg.RegisterAsset(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := NewMediaAssetFromStorage(storageID, teamID, "image/webp", "b", "h2", 20, time.Now().UTC())
	if err := reg.RegisterAsset(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	got, err := reg.FindAsset(context.Background(), teamID, storageID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MimeType != "image/webp" || got.FileSize != 20 || got.ContentHash != "h2" {
		t.Fatalf("expected upserted metadata, got %#v", got)
	}
}

func TestAssetRegistryFindNotFound(t *testing.T) {
	reg := NewAssetRegistry(NewMemoryAssetRepository())
	_, err := reg.FindAsset(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("expected ErrAssetNotFound, got %v", err)
	}
}

func TestAssetRegistryFindRejectsNilAssetID(t *testing.T) {
	reg := NewAssetRegistry(NewMemoryAssetRepository())
	_, err := reg.FindAsset(context.Background(), uuid.New(), uuid.Nil)
	if !errors.Is(err, ErrInvalidAssetID) {
		t.Fatalf("expected ErrInvalidAssetID, got %v", err)
	}
}

func TestMediaReferenceSources(t *testing.T) {
	ref := MediaReference{
		AssetID:         uuid.New(),
		ProviderMediaID: "123456",
		MimeType:        "image/jpeg",
		Source:          MediaSourceCacheHit,
	}
	if !ref.UsesProviderMediaID() || ref.UsesDeprecatedLink() {
		t.Fatalf("unexpected ref %#v", ref)
	}

	linkRef := MediaReference{
		Source:         MediaSourceDeprecatedLink,
		DeprecatedLink: "https://cdn.example.com/x.jpg",
	}
	if !linkRef.UsesDeprecatedLink() || linkRef.UsesProviderMediaID() {
		t.Fatalf("unexpected link ref %#v", linkRef)
	}
}

func TestMemoryMetricsCounters(t *testing.T) {
	m := &MemoryMetrics{}
	var _ Metrics = m
	m.IncCacheHit()
	m.IncCacheMiss()
	m.IncUpload()
	m.IncRefresh()
	m.IncUploadFailure()
	m.IncDeprecatedLink()
	if m.CacheHit != 1 || m.CacheMiss != 1 || m.Upload != 1 || m.Refresh != 1 || m.UploadFailure != 1 || m.DeprecatedLink != 1 {
		t.Fatalf("unexpected metrics %#v", m)
	}
}

// Compile-time check: AssetRegistry covers asset side of Manager.
var _ interface {
	RegisterAsset(context.Context, MediaAsset) error
	FindAsset(context.Context, uuid.UUID, uuid.UUID) (*MediaAsset, error)
} = (*AssetRegistry)(nil)
