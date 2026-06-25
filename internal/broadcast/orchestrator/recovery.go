package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Recovery requeues batches stuck in processing after lease expiry.
type Recovery struct {
	campaigns CampaignStore
	publisher Publisher
	outbox    OutboxStore
	metrics   *Metrics
	lease     time.Duration
	log       *slog.Logger
}

func NewRecovery(
	campaigns CampaignStore,
	publisher Publisher,
	outbox OutboxStore,
	metrics *Metrics,
	lease time.Duration,
	log *slog.Logger,
) *Recovery {
	if log == nil {
		log = slog.Default()
	}
	if lease <= 0 {
		lease = DefaultBatchLease
	}
	return &Recovery{
		campaigns: campaigns,
		publisher: publisher,
		outbox:    outbox,
		metrics:   metrics,
		lease:     lease,
		log:       log,
	}
}

func (r *Recovery) RunOnce(ctx context.Context, now time.Time) error {
	expiredBefore := now.Add(-r.lease)
	batches, err := r.campaigns.ListStaleProcessingBatches(ctx, expiredBefore, 100)
	if err != nil {
		return err
	}
	for i := range batches {
		b := &batches[i]
		if err := r.campaigns.ResetSendingRecipientsWithoutWAMID(ctx, b.ID); err != nil {
			return err
		}
		if err := r.campaigns.RequeueStaleBatch(ctx, b.ID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return err
		}
		job, err := r.campaigns.FindJobByID(ctx, b.JobID)
		if err != nil {
			return err
		}
		b.Status = campaign.BatchQueued
		if err := PublishBatchSend(ctx, r.outbox, r.publisher, b, job); err != nil {
			return err
		}
		r.metrics.IncBatchRecovery()
		r.log.Info("stale batch requeued",
			"campaign_id", b.CampaignID.String(),
			"job_id", b.JobID.String(),
			"batch_id", b.ID.String(),
			"team_id", job.TeamID.String(),
			"channel_id", job.ChannelID.String(),
		)
	}
	return nil
}

func (r *Recovery) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRecoveryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx, time.Now().UTC()); err != nil {
			r.log.Error("batch recovery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
