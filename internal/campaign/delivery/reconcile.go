package delivery

import (
	"context"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

// Reconciler rebuilds campaign counters from recipient status aggregates.
type Reconciler struct {
	campaigns campaign.Repository
	metrics   *Metrics
	log       *slog.Logger
	lookback  time.Duration
}

func NewReconciler(campaigns campaign.Repository, metrics *Metrics, log *slog.Logger, lookback time.Duration) *Reconciler {
	if log == nil {
		log = slog.Default()
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	if lookback <= 0 {
		lookback = 30 * 24 * time.Hour
	}
	return &Reconciler{campaigns: campaigns, metrics: metrics, log: log, lookback: lookback}
}

// Run repairs counter drift for recently updated campaigns.
func (r *Reconciler) Run(ctx context.Context) error {
	since := time.Now().UTC().Add(-r.lookback)
	ids, err := r.campaigns.ListCampaignIDsForReconciliation(ctx, since)
	if err != nil {
		return err
	}
	for _, id := range ids {
		repaired, err := r.campaigns.RepairCampaignCounters(ctx, id)
		if err != nil {
			r.log.Error("campaign reconciliation failed", "campaign_id", id.String(), "error", err)
			continue
		}
		if repaired {
			r.metrics.IncReconciliationRepairs()
			r.log.Info("campaign counters reconciled", "campaign_id", id.String())
		}
	}
	return nil
}
