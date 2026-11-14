package idempotency

import (
	"context"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// PostgresStore is the durable idempotency record (processed_events), shared by
// every replica and kept across restarts.
type PostgresStore struct {
	db *database.DB
}

func NewPostgresStore(db *database.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Seen(ctx context.Context, key string) (bool, error) {
	var seen bool
	err := s.db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM processed_events WHERE key = $1)`, key).Scan(&seen)
	return seen, err
}

func (s *PostgresStore) Mark(ctx context.Context, key string) error {
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO processed_events (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`, key)
	return err
}

// PurgeOlderThan deletes records past the redelivery horizon and returns how many.
func (s *PostgresStore) PurgeOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	tag, err := s.db.Pool.Exec(ctx, `DELETE FROM processed_events WHERE processed_at < $1`, time.Now().Add(-age))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CachedStore reads a fast cache (Redis) first and falls back to the durable
// store; Mark writes the durable store first. Cache failures never decide the
// outcome, so a Redis outage only costs a database round trip.
type CachedStore struct {
	durable Store
	cache   Store
}

func NewCachedStore(durable, cache Store) *CachedStore {
	return &CachedStore{durable: durable, cache: cache}
}

func (s *CachedStore) Seen(ctx context.Context, key string) (bool, error) {
	if s.cache != nil {
		if seen, err := s.cache.Seen(ctx, key); err == nil && seen {
			return true, nil
		}
	}
	seen, err := s.durable.Seen(ctx, key)
	if err == nil && seen && s.cache != nil {
		_ = s.cache.Mark(ctx, key) // warm the cache for the next redelivery
	}
	return seen, err
}

func (s *CachedStore) Mark(ctx context.Context, key string) error {
	if err := s.durable.Mark(ctx, key); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Mark(ctx, key)
	}
	return nil
}
