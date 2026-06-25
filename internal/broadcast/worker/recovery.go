package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

const (
	DefaultRecipientRecoveryInterval = 1 * time.Minute
	DefaultRecipientStaleAfter       = 15 * time.Minute
)

// RecipientRecovery requeues recipients stuck in sending without a WAMID.
type RecipientRecovery struct {
	sends    campaign.SendRepository
	metrics  *Metrics
	stale    time.Duration
	log      *slog.Logger
}

func NewRecipientRecovery(sends campaign.SendRepository, metrics *Metrics, stale time.Duration, log *slog.Logger) *RecipientRecovery {
	if stale <= 0 {
		stale = DefaultRecipientStaleAfter
	}
	if log == nil {
		log = slog.Default()
	}
	return &RecipientRecovery{sends: sends, metrics: metrics, stale: stale, log: log}
}

func (r *RecipientRecovery) RunOnce(ctx context.Context, now time.Time) error {
	staleBefore := now.Add(-r.stale)
	count, err := r.sends.RecoverStaleSendingRecipients(ctx, staleBefore, 500)
	if err != nil {
		return err
	}
	if count > 0 {
		if r.metrics != nil {
			r.metrics.AddRecipientRecovery(int64(count))
		}
		r.log.Info("stale sending recipients requeued", "count", count)
	}
	return nil
}

func (r *RecipientRecovery) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRecipientRecoveryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx, time.Now().UTC()); err != nil {
			r.log.Error("recipient recovery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
