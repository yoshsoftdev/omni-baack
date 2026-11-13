//go:build integration

package inbox_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := database.New(context.Background(), config.DatabaseConfig{URL: url, MaxOpenConns: 20, MaxIdleConns: 5, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// seedConversation inserts the minimum rows a conversation needs and removes
// them (cascading) when the test ends.
func seedConversation(t *testing.T, db *database.DB) (teamID, conversationID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	teamID, contactID, profileID, conversationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO teams (id, name, slug) VALUES ($1, 'it team', $2)`, []any{teamID, "it-" + teamID.String()}},
		{`INSERT INTO contacts (id, team_id, contact_name, customer_name) VALUES ($1, $2, 'it', 'it')`, []any{contactID, teamID}},
		{`INSERT INTO provider_profiles (id, team_id, contact_id, provider, provider_user_id) VALUES ($1, $2, $3, 'whatsapp', $4)`, []any{profileID, teamID, contactID, "628" + contactID.String()[:8]}},
		{`INSERT INTO conversations (id, team_id, contact_id, provider_profile_id, external_thread_id) VALUES ($1, $2, $3, $4, $5)`, []any{conversationID, teamID, contactID, profileID, conversationID.String()}},
	}
	for _, s := range stmts {
		if _, err := db.Pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, s.sql)
		}
	}
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM teams WHERE id = $1`, teamID) })
	return teamID, conversationID
}

// Many API replicas race to claim the same due recontact. The UNIQUE ledger
// constraint must let exactly one of them win.
func TestWindowRecontactClaimIsExclusiveInPostgres(t *testing.T) {
	db := openTestDB(t)
	repo := inbox.NewSettingsRepository(db)
	teamID, conversationID := seedConversation(t, db)
	customerAt := time.Now().UTC().Add(-23 * time.Hour).Truncate(time.Microsecond)

	const replicas = 16
	var winners atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < replicas; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, _, err := repo.TryClaimWindowRecontact(context.Background(), teamID, conversationID, customerAt)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if claimed {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("winners = %d, want exactly 1", got)
	}
}

// A failed send releases its claim; a sent one cannot be released.
func TestWindowRecontactClaimReleaseOnlyWhenUnsent(t *testing.T) {
	db := openTestDB(t)
	repo := inbox.NewSettingsRepository(db)
	ctx := context.Background()
	teamID, conversationID := seedConversation(t, db)
	at := time.Now().UTC().Add(-23 * time.Hour).Truncate(time.Microsecond)

	_, sendID, err := repo.TryClaimWindowRecontact(ctx, teamID, conversationID, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteWindowRecontactClaim(ctx, sendID); err != nil {
		t.Fatal(err)
	}
	claimed, sendID, err := repo.TryClaimWindowRecontact(ctx, teamID, conversationID, at)
	if err != nil || !claimed {
		t.Fatalf("re-claim after release: claimed=%v err=%v", claimed, err)
	}
	if err := repo.MarkWindowRecontactSent(ctx, sendID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteWindowRecontactClaim(ctx, sendID); err != nil {
		t.Fatal(err)
	}
	if claimed, _, _ := repo.TryClaimWindowRecontact(ctx, teamID, conversationID, at); claimed {
		t.Fatal("a sent recontact must stay claimed (no second message)")
	}
}
