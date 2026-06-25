package idempotency

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisPrefix = "bc:idemp:"

// RedisStore uses SET NX for consumer idempotency.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStore(client *redis.Client, ttl time.Duration) *RedisStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisStore{client: client, ttl: ttl}
}

func (s *RedisStore) Seen(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, redisPrefix+key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *RedisStore) Mark(ctx context.Context, key string) error {
	return s.client.SetNX(ctx, redisPrefix+key, "1", s.ttl).Err()
}

// ProcessOnce runs fn when the key has not been seen; marks key after success.
func ProcessOnce(ctx context.Context, store Store, key string, fn func() error) (skipped bool, err error) {
	seen, err := store.Seen(ctx, key)
	if err != nil {
		return false, err
	}
	if seen {
		return true, nil
	}
	if err := fn(); err != nil {
		return false, err
	}
	if markErr := store.Mark(ctx, key); markErr != nil {
		return false, markErr
	}
	return false, nil
}
