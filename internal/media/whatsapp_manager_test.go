package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeObjectStore struct {
	meta    map[uuid.UUID]*ObjectMeta
	content map[uuid.UUID][]byte
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{
		meta:    map[uuid.UUID]*ObjectMeta{},
		content: map[uuid.UUID][]byte{},
	}
}

func (f *fakeObjectStore) put(teamID, id uuid.UUID, mime, key, hash string, body []byte) {
	f.meta[id] = &ObjectMeta{
		ID: id, TeamID: teamID, ObjectKey: key, MimeType: mime, SizeBytes: int64(len(body)), ContentHash: hash,
	}
	f.content[id] = append([]byte(nil), body...)
}

func (f *fakeObjectStore) GetMetadata(_ context.Context, _, objectID uuid.UUID) (*ObjectMeta, error) {
	m, ok := f.meta[objectID]
	if !ok {
		return nil, fmt.Errorf("storage object not found")
	}
	cp := *m
	return &cp, nil
}

func (f *fakeObjectStore) Download(_ context.Context, _, objectID uuid.UUID) (io.ReadCloser, *ObjectMeta, error) {
	m, ok := f.meta[objectID]
	if !ok {
		return nil, nil, fmt.Errorf("storage object not found")
	}
	body := f.content[objectID]
	cp := *m
	return io.NopCloser(bytes.NewReader(body)), &cp, nil
}

type fakeUploader struct {
	mu       sync.Mutex
	calls    int
	failTimes int
	err      error
	mediaID  string
}

func (u *fakeUploader) Upload(context.Context, string, string, string, string, io.Reader, int64) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if u.failTimes > 0 {
		u.failTimes--
		if u.err != nil {
			return "", u.err
		}
		return "", errors.New("temporary 503")
	}
	if u.mediaID == "" {
		u.mediaID = "media-uploaded"
	}
	return u.mediaID, nil
}

func newTestManager(t *testing.T, store *fakeObjectStore, uploader *fakeUploader, metrics *MemoryMetrics) *WhatsAppMediaManager {
	t.Helper()
	mgr, err := NewWhatsAppMediaManager(WhatsAppMediaManagerConfig{
		Assets:      NewMemoryAssetRepository(),
		Cache:       NewMemoryProviderMediaCacheRepository(),
		Storage:     store,
		Uploader:    uploader,
		Policy:      AgeBasedMediaCachePolicy{MaxAge: 28 * 24 * time.Hour},
		Retry:       UploadRetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, IsTransient: DefaultTransientUploadError},
		RefreshWait: RefreshWaitPolicy{MaxWait: 50 * time.Millisecond, Interval: time.Millisecond},
		Metrics:     metrics,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestEnsureMediaCacheHit(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "a.jpg", "h", []byte("jpeg"))
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{mediaID: "should-not-call"}
	mgr := newTestManager(t, store, uploader, metrics)

	if _, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed upload: %v", err)
	}
	uploader.calls = 0
	metrics.CacheHit = 0

	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Source != MediaSourceCacheHit || ref.ProviderMediaID == "" {
		t.Fatalf("unexpected ref %#v", ref)
	}
	if uploader.calls != 0 {
		t.Fatalf("cache hit must not upload, calls=%d", uploader.calls)
	}
	if metrics.CacheHit != 1 {
		t.Fatalf("expected cache hit metric 1, got %d", metrics.CacheHit)
	}
}

func TestEnsureMediaForceRefresh(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/png", "a.png", "h", []byte("png"))
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{mediaID: "id-1"}
	mgr := newTestManager(t, store, uploader, metrics)

	first, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	uploader.mediaID = "id-2"
	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok", ForceRefresh: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Source != MediaSourceRefresh {
		t.Fatalf("expected refresh source, got %s", ref.Source)
	}
	if ref.ProviderMediaID != "id-2" {
		t.Fatalf("expected id-2, got %s", ref.ProviderMediaID)
	}
	if first.ProviderMediaID == ref.ProviderMediaID {
		t.Fatal("force refresh must replace media id")
	}
	if metrics.Refresh < 1 {
		t.Fatalf("expected refresh metric, got %d", metrics.Refresh)
	}
}

func TestEnsureMediaUploadRetryThenSuccess(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "a.jpg", "h", []byte("jpeg"))
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{failTimes: 2, err: errors.New("503 temporary"), mediaID: "ok-id"}
	mgr := newTestManager(t, store, uploader, metrics)

	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ProviderMediaID != "ok-id" {
		t.Fatalf("got %s", ref.ProviderMediaID)
	}
	if uploader.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", uploader.calls)
	}
	if metrics.Upload != 1 || metrics.UploadFailure != 0 {
		t.Fatalf("metrics upload=%d failure=%d", metrics.Upload, metrics.UploadFailure)
	}
}

func TestEnsureMediaPermanentFailureKeepsCache(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "a.jpg", "h", []byte("jpeg"))
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{mediaID: "good"}
	mgr := newTestManager(t, store, uploader, metrics)

	if _, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	}); err != nil {
		t.Fatal(err)
	}

	uploader.failTimes = 10
	uploader.err = errors.New("meta upload error: invalid media type")
	_, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok", ForceRefresh: true,
	})
	if !errors.Is(err, ErrUploadFailed) {
		t.Fatalf("expected ErrUploadFailed, got %v", err)
	}
	if metrics.UploadFailure < 1 {
		t.Fatal("expected upload failure metric")
	}

	entry, err := mgr.cache.FindByKey(context.Background(), CacheKey{
		Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProviderMediaID != "good" {
		t.Fatalf("failure must preserve previous media id, got %s", entry.ProviderMediaID)
	}
	if entry.State != CacheStateFailed {
		t.Fatalf("expected FAILED, got %s", entry.State)
	}
}

func TestEnsureMediaPolicyRefresh(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "a.jpg", "h", []byte("jpeg"))
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{mediaID: "old"}
	mgr := newTestManager(t, store, uploader, metrics)
	mgr.policy = AgeBasedMediaCachePolicy{MaxAge: time.Hour}

	if _, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	}); err != nil {
		t.Fatal(err)
	}

	// Age the cache entry beyond policy.
	entry, err := mgr.cache.FindByKey(context.Background(), CacheKey{
		Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn1",
	})
	if err != nil {
		t.Fatal(err)
	}
	entry.UploadedAt = time.Now().UTC().Add(-2 * time.Hour)
	if err := mgr.cache.Upsert(context.Background(), *entry); err != nil {
		t.Fatal(err)
	}

	uploader.mediaID = "new"
	uploader.calls = 0
	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Source != MediaSourceRefresh || ref.ProviderMediaID != "new" {
		t.Fatalf("unexpected %#v", ref)
	}
	if uploader.calls != 1 {
		t.Fatalf("expected one upload, got %d", uploader.calls)
	}
}

func TestEnsureMediaReturnsMediaReferenceNotString(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/webp", "x.webp", "h", []byte("w"))
	mgr := newTestManager(t, store, &fakeUploader{mediaID: "123"}, &MemoryMetrics{})

	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil || !ref.UsesProviderMediaID() || ref.MimeType != "image/webp" || ref.AssetID != assetID {
		t.Fatalf("unexpected %#v", ref)
	}
}

func TestDefaultTransientUploadError(t *testing.T) {
	if !DefaultTransientUploadError(errors.New("meta upload error: 503 unavailable")) {
		t.Fatal("503 should be transient")
	}
	if DefaultTransientUploadError(errors.New("meta upload error: invalid parameter")) {
		t.Fatal("invalid parameter should be permanent")
	}
	if !DefaultTransientUploadError(fmt.Errorf("wrap: %w", context.DeadlineExceeded)) {
		t.Fatal("deadline exceeded should be transient")
	}
}

func TestEnsureMediaSingleFlightWait(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "a.jpg", "h", []byte("jpeg"))
	cache := NewMemoryProviderMediaCacheRepository()
	metrics := &MemoryMetrics{}
	uploader := &fakeUploader{mediaID: "shared"}

	mgr, err := NewWhatsAppMediaManager(WhatsAppMediaManagerConfig{
		Assets:      NewMemoryAssetRepository(),
		Cache:       cache,
		Storage:     store,
		Uploader:    uploader,
		Policy:      AgeBasedMediaCachePolicy{MaxAge: 28 * 24 * time.Hour},
		Retry:       DefaultUploadRetryPolicy(),
		RefreshWait: RefreshWaitPolicy{MaxWait: 2 * time.Second, Interval: 10 * time.Millisecond},
		Metrics:     metrics,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	entry, claimed, err := cache.ClaimRefresh(context.Background(), teamID, CacheKey{
		Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn1",
	}, now, StaleRefreshLockTimeout)
	if err != nil || !claimed {
		t.Fatalf("claim seed: %v %v", claimed, err)
	}

	done := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		done <- cache.CompleteRefresh(context.Background(), entry.ID, "shared", time.Now().UTC())
	}()

	ref, err := mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn1", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("waiter EnsureMedia: %v", err)
	}
	if ref.ProviderMediaID != "shared" || ref.Source != MediaSourceCacheHit {
		t.Fatalf("unexpected waiter ref %#v", ref)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNewWhatsAppMediaManagerRequiresDeps(t *testing.T) {
	_, err := NewWhatsAppMediaManager(WhatsAppMediaManagerConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("unexpected: %v", err)
	}
}
