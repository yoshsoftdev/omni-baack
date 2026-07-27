package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAgeBasedMediaCachePolicyNeedsRefresh(t *testing.T) {
	policy := DefaultWhatsAppMediaCachePolicy()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	fresh := ProviderMediaCache{
		ProviderMediaID: "111",
		State:           CacheStateActive,
		UploadedAt:      now.Add(-24 * time.Hour),
	}
	if policy.NeedsRefresh(fresh, now) {
		t.Fatal("fresh ACTIVE entry must not need refresh")
	}

	old := ProviderMediaCache{
		ProviderMediaID: "111",
		State:           CacheStateActive,
		UploadedAt:      now.Add(-29 * 24 * time.Hour),
	}
	if !policy.NeedsRefresh(old, now) {
		t.Fatal("entry older than 28 days must need refresh")
	}

	failed := ProviderMediaCache{
		ProviderMediaID: "111",
		State:           CacheStateFailed,
		UploadedAt:      now.Add(-time.Hour),
	}
	if !policy.NeedsRefresh(failed, now) {
		t.Fatal("FAILED entry must need refresh")
	}

	empty := ProviderMediaCache{State: CacheStateActive, UploadedAt: now}
	if !policy.NeedsRefresh(empty, now) {
		t.Fatal("missing provider_media_id must need refresh")
	}
}

func TestProviderMediaCacheUpsertIdempotent(t *testing.T) {
	repo := NewMemoryProviderMediaCacheRepository()
	assetID := uuid.New()
	teamID := uuid.New()
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn-1"}

	first := ProviderMediaCache{
		TeamID:          teamID,
		Provider:        key.Provider,
		AssetID:         key.AssetID,
		PhoneNumberID:   key.PhoneNumberID,
		ProviderMediaID: "media-1",
		State:           CacheStateActive,
		UploadedAt:      time.Now().UTC(),
	}
	if err := repo.Upsert(context.Background(), first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	got, err := repo.FindByKey(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	id := got.ID

	second := first
	second.ID = uuid.Nil
	second.ProviderMediaID = "media-2"
	if err := repo.Upsert(context.Background(), second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err = repo.FindByKey(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id {
		t.Fatalf("upsert must keep same logical row id, got %s want %s", got.ID, id)
	}
	if got.ProviderMediaID != "media-2" {
		t.Fatalf("expected media-2, got %s", got.ProviderMediaID)
	}
}

func TestClaimRefreshSingleFlight(t *testing.T) {
	repo := NewMemoryProviderMediaCacheRepository()
	teamID := uuid.New()
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: uuid.New(), PhoneNumberID: "pn-1"}
	now := time.Now().UTC()

	first, claimed, err := repo.ClaimRefresh(context.Background(), teamID, key, now, StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	if first.State != CacheStateRefreshing {
		t.Fatalf("expected REFRESHING, got %s", first.State)
	}

	second, claimed, err := repo.ClaimRefresh(context.Background(), teamID, key, now.Add(time.Second), StaleRefreshLockTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("second claim must not win while lock is fresh")
	}
	if second.ID != first.ID {
		t.Fatal("second claim should return same row")
	}
}

func TestClaimRefreshReclaimsStaleLock(t *testing.T) {
	repo := NewMemoryProviderMediaCacheRepository()
	teamID := uuid.New()
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: uuid.New(), PhoneNumberID: "pn-1"}
	started := time.Now().UTC().Add(-10 * time.Minute)

	_, claimed, err := repo.ClaimRefresh(context.Background(), teamID, key, started, StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatalf("seed claim: %v claimed=%v", err, claimed)
	}

	now := time.Now().UTC()
	entry, claimed, err := repo.ClaimRefresh(context.Background(), teamID, key, now, StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatalf("stale reclaim: claimed=%v err=%v", claimed, err)
	}
	if entry.State != CacheStateRefreshing {
		t.Fatalf("expected REFRESHING, got %s", entry.State)
	}
}

func TestCompleteRefreshAndFailRefreshPreserveMediaID(t *testing.T) {
	repo := NewMemoryProviderMediaCacheRepository()
	teamID := uuid.New()
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: uuid.New(), PhoneNumberID: "pn-1"}
	now := time.Now().UTC()

	entry, claimed, err := repo.ClaimRefresh(context.Background(), teamID, key, now, StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	if err := repo.CompleteRefresh(context.Background(), entry.ID, "999", now); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByKey(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != CacheStateActive || got.ProviderMediaID != "999" {
		t.Fatalf("unexpected after complete %#v", got)
	}

	_, claimed, err = repo.ClaimRefresh(context.Background(), teamID, key, now.Add(time.Minute), StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	if err := repo.FailRefresh(context.Background(), got.ID, "failed to upload media to Meta", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByKey(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != CacheStateFailed {
		t.Fatalf("expected FAILED, got %s", got.State)
	}
	if got.ProviderMediaID != "999" {
		t.Fatalf("fail must preserve previous media id, got %s", got.ProviderMediaID)
	}
	if got.LastError == nil || *got.LastError == "" {
		t.Fatal("expected last_error")
	}
}

func TestFindByKeyNotFound(t *testing.T) {
	repo := NewMemoryProviderMediaCacheRepository()
	_, err := repo.FindByKey(context.Background(), CacheKey{
		Provider: ProviderWhatsApp, AssetID: uuid.New(), PhoneNumberID: "x",
	})
	if !errors.Is(err, ErrCacheNotFound) {
		t.Fatalf("expected ErrCacheNotFound, got %v", err)
	}
}

var _ ProviderMediaCacheRepository = (*MemoryProviderMediaCacheRepository)(nil)
var _ ProviderMediaCacheRepository = (*PostgresProviderMediaCacheRepository)(nil)
var _ MediaCachePolicy = AgeBasedMediaCachePolicy{}
