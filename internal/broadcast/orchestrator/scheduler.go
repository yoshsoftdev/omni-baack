package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Scheduler dispatches due scheduled campaigns into the orchestration pipeline.
type Scheduler struct {
	campaigns CampaignStore
	outbox    OutboxStore
	metrics   *Metrics
	log       *slog.Logger
}

func NewScheduler(campaigns CampaignStore, outbox OutboxStore, metrics *Metrics, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{campaigns: campaigns, outbox: outbox, metrics: metrics, log: log}
}

// Tick claims due scheduled campaigns and queues them for orchestration.
// Idempotency: Postgres SKIP LOCKED + unique active job index + status transition guard.
func (s *Scheduler) Tick(ctx context.Context, now time.Time, limit int) error {
	due, err := s.campaigns.ClaimDueScheduled(ctx, now, limit)
	if err != nil {
		return err
	}
	for i := range due {
		camp := &due[i]
		if err := s.dispatchOne(ctx, camp); err != nil {
			s.log.Error("scheduler dispatch failed",
				"campaign_id", camp.ID.String(),
				"team_id", camp.TeamID.String(),
				"error", err,
			)
		}
	}
	return nil
}

func (s *Scheduler) dispatchOne(ctx context.Context, camp *campaign.Campaign) error {
	if err := s.campaigns.TransitionCampaignToQueued(ctx, camp.TeamID, camp.ID); err != nil {
		if err == repository.ErrNotFound {
			return nil
		}
		return err
	}

	job, err := s.campaigns.FindActiveJobByCampaign(ctx, camp.ID)
	if err != nil {
		if err == repository.ErrNotFound {
			now := time.Now().UTC()
			job = &campaign.BroadcastJob{
				ID:         uuid.New(),
				CampaignID: camp.ID,
				TeamID:     camp.TeamID,
				ChannelID:  camp.ChannelID,
				Status:     campaign.JobPending,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := s.campaigns.CreateJob(ctx, job); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if err := EnqueueJobCreated(ctx, s.outbox, job); err != nil {
		return err
	}
	s.metrics.IncSchedulerCampaigns()
	s.log.Info("scheduled campaign queued",
		"campaign_id", camp.ID.String(),
		"job_id", job.ID.String(),
		"team_id", camp.TeamID.String(),
		"channel_id", camp.ChannelID.String(),
	)
	return nil
}
