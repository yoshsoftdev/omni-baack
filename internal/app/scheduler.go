package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	eventconsumeridempotency "github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/leader"
)

// processedEventsRetention must exceed the JetStream stream MaxAge (7 days):
// older events can no longer be redelivered, so their records are dead weight.
const processedEventsRetention = 8 * 24 * time.Hour

// startScheduler starts every periodic job; each tick runs on one replica only.
func startScheduler(ctx context.Context, db *database.DB, log *slog.Logger, recontact *inbox.WindowRecontactService, reconciler *delivery.Reconciler) {
	go leader.Every(ctx, db, log, "window-recontact", leader.LockWindowRecontact, recontact.Interval(), recontact.RunOnce)

	go leader.Every(ctx, db, log, "delivery-reconcile", leader.LockDeliveryReconcile, 24*time.Hour, func(ctx context.Context) {
		if err := reconciler.Run(ctx); err != nil {
			log.Error("delivery reconciliation failed", "error", err)
		}
	})

	processed := eventconsumeridempotency.NewPostgresStore(db)
	go leader.Every(ctx, db, log, "purge-processed-events", leader.LockPurgeProcessedEvents, time.Hour, func(ctx context.Context) {
		if n, err := processed.PurgeOlderThan(ctx, processedEventsRetention); err != nil {
			log.Warn("purge processed_events failed", "error", err)
		} else if n > 0 {
			log.Info("purged processed_events", "rows", n)
		}
	})
}
