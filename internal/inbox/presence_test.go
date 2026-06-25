package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
)

type memPresenceRepo struct {
	last *inbox.MemberPresence
}

func (m *memPresenceRepo) Upsert(_ context.Context, p *inbox.MemberPresence) error {
	cp := *p
	m.last = &cp
	return nil
}
func (m *memPresenceRepo) ListByTeam(context.Context, uuid.UUID) ([]inbox.MemberPresence, error) {
	return nil, nil
}

func TestPresenceOnlineOffline(t *testing.T) {
	repo := &memPresenceRepo{}
	svc := inbox.NewPresenceService(repo)
	teamID, userID := uuid.New(), uuid.New()
	if err := svc.SetOnline(context.Background(), teamID, userID); err != nil {
		t.Fatal(err)
	}
	if repo.last.Status != inbox.PresenceOnline {
		t.Fatalf("status %s", repo.last.Status)
	}
	if err := svc.SetOffline(context.Background(), teamID, userID); err != nil {
		t.Fatal(err)
	}
	if repo.last.Status != inbox.PresenceOffline {
		t.Fatalf("status %s", repo.last.Status)
	}
	if repo.last.LastSeenAt.IsZero() {
		t.Fatal("expected last_seen_at")
	}
	_ = time.Now()
}
