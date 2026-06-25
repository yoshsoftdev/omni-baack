package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrRateLimited indicates the channel throughput cap was reached.
var ErrRateLimited = errors.New("channel rate limited")

// RateLimiter gates outbound sends per channel.
type RateLimiter interface {
	Acquire(ctx context.Context, channelID uuid.UUID) error
}

// MemoryRateLimiter enforces per-channel limits in-process (1 channel = 1 limiter).
type MemoryRateLimiter struct {
	mu      sync.Mutex
	limit   int
	buckets map[uuid.UUID]*rateBucket
}

type rateBucket struct {
	count  int
	window time.Time
}

func NewMemoryRateLimiter(limitPerSec int) *MemoryRateLimiter {
	if limitPerSec <= 0 {
		limitPerSec = DefaultRateLimitPerSec
	}
	return &MemoryRateLimiter{limit: limitPerSec, buckets: make(map[uuid.UUID]*rateBucket)}
}

func (l *MemoryRateLimiter) Acquire(_ context.Context, channelID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[channelID]
	if b == nil || now.Sub(b.window) >= time.Second {
		b = &rateBucket{window: now}
		l.buckets[channelID] = b
	}
	if b.count >= l.limit {
		return ErrRateLimited
	}
	b.count++
	return nil
}

// RedisTokenBucketLimiter enforces shared per-channel limits using Redis.
type RedisTokenBucketLimiter struct {
	client *redis.Client
	limit  int
}

func NewRedisTokenBucketLimiter(client *redis.Client, limitPerSec int) *RedisTokenBucketLimiter {
	if limitPerSec <= 0 {
		limitPerSec = DefaultRateLimitPerSec
	}
	return &RedisTokenBucketLimiter{client: client, limit: limitPerSec}
}

func (l *RedisTokenBucketLimiter) key(channelID uuid.UUID) string {
	return "broadcast:rate:" + channelID.String()
}

func (l *RedisTokenBucketLimiter) Acquire(ctx context.Context, channelID uuid.UUID) error {
	if l.client == nil {
		return nil
	}
	key := l.key(channelID)
	n, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if n == 1 {
		_ = l.client.Expire(ctx, key, time.Second).Err()
	}
	if int(n) > l.limit {
		return ErrRateLimited
	}
	return nil
}

// FallbackRateLimiter uses Redis when healthy and falls back to in-process limiting.
type FallbackRateLimiter struct {
	primary  RateLimiter
	fallback RateLimiter
	metrics  *Metrics
}

func NewFallbackRateLimiter(primary, fallback RateLimiter, metrics *Metrics) *FallbackRateLimiter {
	return &FallbackRateLimiter{primary: primary, fallback: fallback, metrics: metrics}
}

func (l *FallbackRateLimiter) Acquire(ctx context.Context, channelID uuid.UUID) error {
	if l.primary == nil {
		return l.fallback.Acquire(ctx, channelID)
	}
	err := l.primary.Acquire(ctx, channelID)
	if err == nil || errors.Is(err, ErrRateLimited) {
		return err
	}
	if l.metrics != nil {
		l.metrics.IncRateLimitFallback(channelID)
	}
	if l.fallback == nil {
		return err
	}
	return l.fallback.Acquire(ctx, channelID)
}

// Deprecated: use RedisTokenBucketLimiter.
type RedisRateLimiter = RedisTokenBucketLimiter

// Deprecated: use NewRedisTokenBucketLimiter.
func NewRedisRateLimiter(client *redis.Client, limitPerSec int) *RedisTokenBucketLimiter {
	return NewRedisTokenBucketLimiter(client, limitPerSec)
}
