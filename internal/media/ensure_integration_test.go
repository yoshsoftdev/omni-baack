package media

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Sprint 2 Phase 5 DoD: integration coverage for cache hit/miss/refresh/failure/concurrent.
func TestEnsureMediaIntegrationLifecycle(t *testing.T) {
	store := newFakeObjectStore()
	teamID := uuid.New()
	assetID := uuid.New()
	store.put(teamID, assetID, "image/jpeg", "promo.jpg", "hash", []byte("jpeg-bytes"))

	uploader := &fakeUploader{mediaID: "media-1"}
	metrics := &MemoryMetrics{}
	mgr := newTestManager(t, store, uploader, metrics)

	in := EnsureInput{TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn-1", AccessToken: "tok"}

	// Miss
	ref, err := mgr.EnsureMedia(context.Background(), in)
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if ref.Source != MediaSourceCacheMissUpload || ref.ProviderMediaID != "media-1" {
		t.Fatalf("miss ref %#v", ref)
	}

	// Hit
	uploader.calls = 0
	ref, err = mgr.EnsureMedia(context.Background(), in)
	if err != nil {
		t.Fatalf("hit: %v", err)
	}
	if ref.Source != MediaSourceCacheHit || uploader.calls != 0 {
		t.Fatalf("hit ref %#v calls=%d", ref, uploader.calls)
	}

	// Refresh
	uploader.mediaID = "media-2"
	ref, err = mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn-1", AccessToken: "tok", ForceRefresh: true,
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if ref.Source != MediaSourceRefresh || ref.ProviderMediaID != "media-2" {
		t.Fatalf("refresh ref %#v", ref)
	}

	// Failure preserves previous id
	uploader.failTimes = 5
	uploader.err = errPermanentUpload
	_, err = mgr.EnsureMedia(context.Background(), EnsureInput{
		TeamID: teamID, AssetID: assetID, PhoneNumberID: "pn-1", AccessToken: "tok", ForceRefresh: true,
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	entry, err := mgr.cache.FindByKey(context.Background(), CacheKey{
		Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProviderMediaID != "media-2" || entry.State != CacheStateFailed {
		t.Fatalf("expected preserved id in FAILED state, got %#v", entry)
	}

	// Concurrent refresh after aging cache — single-flight one upload
	uploader.failTimes = 0
	uploader.err = nil
	uploader.mediaID = "media-3"
	uploader.calls = 0
	entry.UploadedAt = time.Now().UTC().Add(-48 * time.Hour)
	entry.State = CacheStateActive
	if err := mgr.cache.Upsert(context.Background(), *entry); err != nil {
		t.Fatal(err)
	}
	mgr.policy = AgeBasedMediaCachePolicy{MaxAge: time.Hour}

	var wg sync.WaitGroup
	errs := make(chan error, 12)
	ids := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := mgr.EnsureMedia(context.Background(), in)
			if e != nil {
				errs <- e
				return
			}
			ids <- r.ProviderMediaID
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		t.Fatalf("concurrent: %v", e)
	}
	for id := range ids {
		if id != "media-3" {
			t.Fatalf("expected media-3, got %s", id)
		}
	}
	if uploader.calls != 1 {
		t.Fatalf("expected single-flight 1 upload, got %d", uploader.calls)
	}
}

var errPermanentUpload = errString("meta upload error: invalid media")

type errString string

func (e errString) Error() string { return string(e) }
