//go:build integration

package idempotency

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

func openIdempotencyDB(t *testing.T) *database.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := database.New(context.Background(), config.DatabaseConfig{URL: url, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// D8: a key marked by one process (or before a restart) is seen by another store instance.
func TestPostgresStoreSurvivesRestart(t *testing.T) {
	db := openIdempotencyDB(t)
	ctx := context.Background()
	key := "it:message:" + uuid.NewString()
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM processed_events WHERE key = $1`, key) })

	if err := NewPostgresStore(db).Mark(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := NewPostgresStore(db).Mark(ctx, key); err != nil {
		t.Fatalf("second mark must be a no-op: %v", err)
	}
	seen, err := NewPostgresStore(db).Seen(ctx, key)
	if err != nil || !seen {
		t.Fatalf("want seen after restart, got %v %v", seen, err)
	}
}

func TestPostgresStorePurgeOlderThan(t *testing.T) {
	db := openIdempotencyDB(t)
	ctx := context.Background()
	old, fresh := "it:old:"+uuid.NewString(), "it:fresh:"+uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM processed_events WHERE key = ANY($1)`, []string{old, fresh})
	})
	if _, err := db.Pool.Exec(ctx, `INSERT INTO processed_events (key, processed_at) VALUES ($1, NOW() - INTERVAL '10 days'), ($2, NOW())`, old, fresh); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(db)
	if _, err := store.PurgeOlderThan(ctx, 8*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if seen, _ := store.Seen(ctx, old); seen {
		t.Fatal("old record should be purged")
	}
	if seen, _ := store.Seen(ctx, fresh); !seen {
		t.Fatal("fresh record must stay")
	}
}
