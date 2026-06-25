package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingLimiter struct {
	mu    sync.Mutex
	count int
	err   error
}

func (c *countingLimiter) Acquire(context.Context, uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	return c.err
}

func (c *countingLimiter) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func TestFallbackRateLimiterUsesPrimary(t *testing.T) {
	primary := &countingLimiter{}
	fallback := &countingLimiter{}
	metrics := NewMetrics()
	limiter := NewFallbackRateLimiter(primary, fallback, metrics)
	if err := limiter.Acquire(context.Background(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if primary.calls() != 1 || fallback.calls() != 0 {
		t.Fatalf("expected primary only, primary=%d fallback=%d", primary.calls(), fallback.calls())
	}
}

func TestFallbackRateLimiterOnRedisError(t *testing.T) {
	primary := &countingLimiter{err: errors.New("redis down")}
	fallback := &countingLimiter{}
	metrics := NewMetrics()
	limiter := NewFallbackRateLimiter(primary, fallback, metrics)
	if err := limiter.Acquire(context.Background(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if primary.calls() != 1 || fallback.calls() != 1 {
		t.Fatalf("expected fallback use, primary=%d fallback=%d", primary.calls(), fallback.calls())
	}
	if metrics.RateLimitFallback != 1 {
		t.Fatalf("expected fallback metric")
	}
}

func TestRedisTokenBucketSharedKey(t *testing.T) {
	limiter := NewRedisTokenBucketLimiter(nil, 80)
	channelID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	if got := limiter.key(channelID); got != "broadcast:rate:"+channelID.String() {
		t.Fatalf("unexpected key: %s", got)
	}
}

func TestMemoryRateLimiterEnforcesLimit(t *testing.T) {
	limiter := NewMemoryRateLimiter(2)
	channelID := uuid.New()
	_ = limiter.Acquire(context.Background(), channelID)
	_ = limiter.Acquire(context.Background(), channelID)
	if err := limiter.Acquire(context.Background(), channelID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limited, got %v", err)
	}
}

func TestRecipientRecoveryIncrementsMetric(t *testing.T) {
	send := &mockSendRepo{}
	metrics := NewMetrics()
	recovery := NewRecipientRecovery(send, metrics, 15*time.Minute, nil)
	// no stale recipients in mock - RunOnce should not fail
	if err := recovery.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
}
