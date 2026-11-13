//go:build integration

package media

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

func openMediaTestDB(t *testing.T) (*database.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{URL: url, MaxOpenConns: 20, MaxIdleConns: 5, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	teamID, assetID := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO teams (id, name, slug) VALUES ($1, 'it team', $2)`, teamID, "it-"+teamID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO media_assets (id, team_id, mime_type, file_size, storage_path, content_hash) VALUES ($1, $2, 'image/jpeg', 4, 'it/a.jpg', 'h')`, assetID, teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM teams WHERE id = $1`, teamID) })
	return db, teamID, assetID
}

// Concurrent first sends of the same media (no cache row yet): exactly one caller
// owns the upload and nobody gets an error.
func TestClaimRefreshFirstUseIsExclusiveInPostgres(t *testing.T) {
	db, teamID, assetID := openMediaTestDB(t)
	repo := NewPostgresProviderMediaCacheRepository(db)
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn-it"}

	const callers = 16
	var mu sync.Mutex
	claimed, errs := 0, []error{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, err := repo.ClaimRefresh(context.Background(), teamID, key, time.Now().UTC(), StaleRefreshLockTimeout)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			} else if ok {
				claimed++
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("%d caller(s) failed instead of waiting: %v", len(errs), errs[0])
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want exactly 1", claimed)
	}
}

// After a completed refresh, a later claim reports the row was ACTIVE so the
// manager can reuse it instead of uploading again.
func TestClaimRefreshReportsPriorStateInPostgres(t *testing.T) {
	db, teamID, assetID := openMediaTestDB(t)
	repo := NewPostgresProviderMediaCacheRepository(db)
	ctx := context.Background()
	key := CacheKey{Provider: ProviderWhatsApp, AssetID: assetID, PhoneNumberID: "pn-it"}

	entry, ok, err := repo.ClaimRefresh(ctx, teamID, key, time.Now().UTC(), StaleRefreshLockTimeout)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if err := repo.CompleteRefresh(ctx, entry.ID, "media-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	again, ok, err := repo.ClaimRefresh(ctx, teamID, key, time.Now().UTC(), StaleRefreshLockTimeout)
	if err != nil || !ok {
		t.Fatalf("second claim: ok=%v err=%v", ok, err)
	}
	if again.ClaimedFromState != CacheStateActive || again.ProviderMediaID != "media-1" {
		t.Fatalf("claimed from %q with id %q, want ACTIVE media-1", again.ClaimedFromState, again.ProviderMediaID)
	}
}
