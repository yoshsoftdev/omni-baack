package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

// BatchPlanner chunks pending recipients into batches and publishes send work.
type BatchPlanner struct {
	campaigns CampaignStore
	publisher Publisher
	outbox    OutboxStore
	metrics   *Metrics
	batchSize int
	log       *slog.Logger
}

func NewBatchPlanner(
	campaigns CampaignStore,
	publisher Publisher,
	outbox OutboxStore,
	metrics *Metrics,
	batchSize int,
	log *slog.Logger,
) *BatchPlanner {
	if log == nil {
		log = slog.Default()
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &BatchPlanner{
		campaigns: campaigns,
		publisher: publisher,
		outbox:    outbox,
		metrics:   metrics,
		batchSize: batchSize,
		log:       log,
	}
}

func (p *BatchPlanner) Plan(ctx context.Context, job *campaign.BroadcastJob) error {
	existing, err := p.campaigns.CountBatchesByJob(ctx, job.ID)
	if err != nil {
		return err
	}

	var batches []campaign.BroadcastBatch
	if existing > 0 {
		batches, err = p.campaigns.ListBatchesByJob(ctx, job.ID)
		if err != nil {
			return err
		}
	} else {
		job.Status = campaign.JobPlanningBatches
		job.UpdatedAt = time.Now().UTC()
		if err := p.campaigns.UpdateJob(ctx, job); err != nil {
			return err
		}
		batches, err = p.campaigns.PlanBatchesForJob(ctx, job.ID, p.batchSize)
		if err != nil {
			return err
		}
		if len(batches) > 0 {
			p.metrics.AddBatchesCreated(int64(len(batches)))
		}
	}

	if len(batches) == 0 {
		return nil
	}

	batchIDs := make([]uuid.UUID, 0, len(batches))
	for i := range batches {
		b := &batches[i]
		if b.Status == campaign.BatchPending {
			batchIDs = append(batchIDs, b.ID)
		}
	}
	if len(batchIDs) > 0 {
		if err := p.campaigns.MarkBatchesQueued(ctx, batchIDs); err != nil {
			return err
		}
		for i := range batches {
			if batches[i].Status == campaign.BatchPending {
				batches[i].Status = campaign.BatchQueued
			}
		}
	}

	for i := range batches {
		b := &batches[i]
		if b.Status != campaign.BatchQueued {
			continue
		}
		if err := PublishBatchSend(ctx, p.outbox, p.publisher, b, job); err != nil {
			return err
		}
		p.log.Info("batch send published",
			"campaign_id", job.CampaignID.String(),
			"job_id", job.ID.String(),
			"batch_id", b.ID.String(),
			"team_id", job.TeamID.String(),
			"channel_id", job.ChannelID.String(),
			"recipient_count", b.RecipientCount,
		)
	}
	return nil
}
