//go:build integration

package message

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func openMessageTestDB(t *testing.T) (*database.DB, uuid.UUID, uuid.UUID) {
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
	teamID, contactID, profileID, conversationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO teams (id, name, slug) VALUES ($1, 'it team', $2)`, []any{teamID, "it-" + teamID.String()}},
		{`INSERT INTO contacts (id, team_id, contact_name, customer_name) VALUES ($1, $2, 'it', 'it')`, []any{contactID, teamID}},
		{`INSERT INTO provider_profiles (id, team_id, contact_id, provider, provider_user_id) VALUES ($1, $2, $3, 'whatsapp', $4)`, []any{profileID, teamID, contactID, "628" + contactID.String()[:8]}},
		{`INSERT INTO conversations (id, team_id, contact_id, provider_profile_id, external_thread_id) VALUES ($1, $2, $3, $4, $5)`, []any{conversationID, teamID, contactID, profileID, conversationID.String()}},
	} {
		if _, err := db.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool.Exec(bg, `DELETE FROM messages WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM conversations WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM provider_profiles WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM contacts WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM teams WHERE id = $1`, teamID)
	})
	return db, teamID, conversationID
}

func inboundMessage(teamID, conversationID uuid.UUID, wamid string) *Message {
	now := time.Now().UTC()
	return &Message{
		ID: uuid.New(), ConversationID: conversationID, TeamID: teamID,
		Direction: DirectionInbound, Sender: "628111", Content: "hi", ContentType: ContentText,
		WAMID: &wamid, ReceivedAt: now, CreatedAt: now,
	}
}

// D7: the same wamid delivered concurrently (redelivery, replicas) is stored once.
func TestCreateRejectsDuplicateWAMIDConcurrently(t *testing.T) {
	db, teamID, conversationID := openMessageTestDB(t)
	repo := NewRepository(db)
	wamid := "wamid.it-" + uuid.NewString()

	const callers = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := repo.Create(context.Background(), inboundMessage(teamID, conversationID, wamid))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, repository.ErrConflict):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok != 1 || conflicts != callers-1 {
		t.Fatalf("want 1 insert and %d conflicts, got %d and %d", callers-1, ok, conflicts)
	}
	var rows int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM messages WHERE team_id = $1 AND wamid = $2`, teamID, wamid).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("want 1 message row, got %d", rows)
	}
}

// A failed message insert must not leave the wamid claimed, or the retry would be dropped.
func TestCreateReleasesWAMIDClaimWhenInsertFails(t *testing.T) {
	db, teamID, conversationID := openMessageTestDB(t)
	repo := NewRepository(db)
	wamid := "wamid.it-" + uuid.NewString()

	bad := inboundMessage(teamID, uuid.New(), wamid) // unknown conversation -> FK violation
	if err := repo.Create(context.Background(), bad); err == nil {
		t.Fatal("expected FK error")
	}
	if err := repo.Create(context.Background(), inboundMessage(teamID, conversationID, wamid)); err != nil {
		t.Fatalf("retry after failed insert: %v", err)
	}
}

// Messages without a wamid (e.g. outbound before Meta answers) are never deduplicated.
func TestCreateAllowsMessagesWithoutWAMID(t *testing.T) {
	db, teamID, conversationID := openMessageTestDB(t)
	repo := NewRepository(db)
	for i := 0; i < 2; i++ {
		m := inboundMessage(teamID, conversationID, "")
		m.WAMID = nil
		if err := repo.Create(context.Background(), m); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
}
