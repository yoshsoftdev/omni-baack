package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingStore struct{}

func (failingStore) Seen(context.Context, string) (bool, error) {
	return false, errors.New("redis down")
}
func (failingStore) Mark(context.Context, string) error { return errors.New("redis down") }

// D8: a Redis outage must not turn into "not seen" or a lost mark.
func TestCachedStoreFallsBackToDurableWhenCacheFails(t *testing.T) {
	ctx := context.Background()
	durable := NewMemoryStore(time.Hour)
	s := NewCachedStore(durable, failingStore{})

	if err := s.Mark(ctx, "k"); err != nil {
		t.Fatalf("mark must succeed when only the cache fails: %v", err)
	}
	seen, err := s.Seen(ctx, "k")
	if err != nil || !seen {
		t.Fatalf("want seen from durable store, got %v %v", seen, err)
	}
}

func TestCachedStoreWarmsCacheFromDurable(t *testing.T) {
	ctx := context.Background()
	durable, cache := NewMemoryStore(time.Hour), NewMemoryStore(time.Hour)
	_ = durable.Mark(ctx, "k") // e.g. marked by another replica

	if seen, _ := NewCachedStore(durable, cache).Seen(ctx, "k"); !seen {
		t.Fatal("want seen")
	}
	if seen, _ := cache.Seen(ctx, "k"); !seen {
		t.Fatal("cache should be warmed after a durable hit")
	}
}

func TestCachedStoreMarkFailsWhenDurableFails(t *testing.T) {
	if err := NewCachedStore(failingStore{}, NewMemoryStore(time.Hour)).Mark(context.Background(), "k"); err == nil {
		t.Fatal("a durable write failure must be returned so the event is redelivered")
	}
}
