// Package leader runs periodic jobs on at most one replica at a time.
package leader

import (
	"context"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// Job lock keys (pg advisory locks, cluster-wide). Never reuse a value.
const (
	LockWindowRecontact      int64 = 0x6f6d6e6901
	LockDeliveryReconcile    int64 = 0x6f6d6e6902
	LockPurgeProcessedEvents int64 = 0x6f6d6e6903
)

// RunExclusive runs fn only if this process wins the advisory lock; otherwise
// another replica is already running the job and this tick is skipped.
// The lock is held on one pooled connection for the duration of fn.
func RunExclusive(ctx context.Context, db *database.DB, key int64, fn func(context.Context)) (ran bool, err error) {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()

	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}
	defer func() {
		// Unlock on a fresh context: ctx may already be cancelled at shutdown.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, key)
	}()
	fn(ctx)
	return true, nil
}

// Every runs fn immediately and then every interval, each run guarded by RunExclusive.
func Every(ctx context.Context, db *database.DB, log *slog.Logger, name string, key int64, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := RunExclusive(ctx, db, key, fn); err != nil && ctx.Err() == nil {
			log.Warn("scheduled job lock failed", "job", name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
