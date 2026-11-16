//go:build integration

package inbox_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
)

// 9.2: welcome settings and keyword rules round-trip, and concurrent consumers claiming the
// same inbound message's auto-reply produce exactly one send.
func TestAutoReplyRepositoryInPostgres(t *testing.T) {
	db := openTestDB(t)
	repo := inbox.NewSettingsRepository(db)
	ctx := context.Background()
	teamID, conversationID := seedConversation(t, db)

	if err := repo.UpsertWelcomeSettings(ctx, teamID, inbox.WelcomeSettings{Enabled: true, Text: "Halo!"}); err != nil {
		t.Fatal(err)
	}
	welcome, err := repo.GetWelcomeSettings(ctx, teamID)
	if err != nil || !welcome.Enabled || welcome.Text != "Halo!" {
		t.Fatalf("welcome = %+v, %v", welcome, err)
	}

	second := inbox.KeywordRule{ID: uuid.New(), Keywords: []string{"harga"}, MatchType: inbox.MatchContains, ReplyText: "b", Enabled: true, SortOrder: 2}
	first := inbox.KeywordRule{ID: uuid.New(), Keywords: []string{"jam", "buka"}, MatchType: inbox.MatchExact, ReplyText: "a", Enabled: true, SortOrder: 1}
	for _, r := range []*inbox.KeywordRule{&second, &first} {
		if err := repo.CreateKeywordRule(ctx, teamID, r); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := repo.ListKeywordRules(ctx, teamID)
	if err != nil || len(rules) != 2 || rules[0].ID != first.ID || len(rules[0].Keywords) != 2 {
		t.Fatalf("rules = %+v, %v (want sort order 1 first)", rules, err)
	}
	first.ReplyText = "updated"
	if err := repo.UpdateKeywordRule(ctx, teamID, &first); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateKeywordRule(ctx, uuid.New(), &first); err == nil {
		t.Fatal("updating another team's rule must fail")
	}
	if err := repo.DeleteKeywordRule(ctx, teamID, second.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountKeywordRules(ctx, teamID); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	inboundID := uuid.New()
	var winners atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, _, err := repo.TryClaimAutoReply(ctx, teamID, conversationID, inboundID, inbox.AutoReplyKeyword, &first.ID)
			if err != nil {
				t.Error(err)
				return
			}
			if claimed {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("claims won = %d, want exactly 1", winners.Load())
	}
	// A different kind for the same message is its own slot.
	if claimed, _, err := repo.TryClaimAutoReply(ctx, teamID, conversationID, inboundID, inbox.AutoReplyWelcome, nil); err != nil || !claimed {
		t.Fatalf("welcome claim = %v, %v", claimed, err)
	}
}
