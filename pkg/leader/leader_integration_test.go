//go:build integration

package leader

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// Two schedulers (replicas) ticking at the same moment: the job body never overlaps.
func TestRunExclusiveAllowsOneRunnerAtATime(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	newDB := func() *database.DB {
		db, err := database.New(ctx, config.DatabaseConfig{URL: url, MaxOpenConns: 4, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(db.Close)
		return db
	}
	replicaA, replicaB := newDB(), newDB()
	const key int64 = 0x7465737401

	started, release := make(chan struct{}), make(chan struct{})
	var running atomic.Int32
	done := make(chan bool)
	go func() {
		ran, err := RunExclusive(ctx, replicaA, key, func(context.Context) {
			running.Add(1)
			close(started)
			<-release
		})
		if err != nil {
			t.Error(err)
		}
		done <- ran
	}()
	<-started

	ranB, err := RunExclusive(ctx, replicaB, key, func(context.Context) { running.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	if ranB {
		t.Fatal("second replica ran while the first held the lock")
	}
	close(release)
	if !<-done {
		t.Fatal("first replica should have run")
	}

	// Lock is released after the run, so the next tick on replica B proceeds.
	ranB, err = RunExclusive(ctx, replicaB, key, func(context.Context) { running.Add(1) })
	if err != nil || !ranB {
		t.Fatalf("lock not released after run: ran=%v err=%v", ranB, err)
	}
	if running.Load() != 2 {
		t.Fatalf("job ran %d times, want 2", running.Load())
	}
}
