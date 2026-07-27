package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

// CampaignStore is the persistence surface for orchestration.
type CampaignStore interface {
	campaign.Repository
	campaign.OrchestrationRepository
}

// AudienceResolver advances jobs whose recipient snapshots were frozen at create/schedule.
// It must not resolve Contact.Phone or query Canonical Provider.
type AudienceResolver struct {
	campaigns CampaignStore
	metrics   *Metrics
	log       *slog.Logger
}

func NewAudienceResolver(
	campaigns CampaignStore,
	metrics *Metrics,
	log *slog.Logger,
) *AudienceResolver {
	if log == nil {
		log = slog.Default()
	}
	return &AudienceResolver{
		campaigns: campaigns,
		metrics:   metrics,
		log:       log,
	}
}

func (r *AudienceResolver) Resolve(ctx context.Context, job *campaign.BroadcastJob) error {
	switch job.Status {
	case campaign.JobPlanningBatches, campaign.JobCompleted, campaign.JobFailed, campaign.JobCancelled:
		return nil
	}

	count, err := r.campaigns.CountRecipientsByJob(ctx, job.ID)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("job %s has no recipient snapshots; recreate the campaign", job.ID)
	}

	now := time.Now().UTC()
	job.Status = campaign.JobResolvingAudience
	job.UpdatedAt = now
	if err := r.campaigns.UpdateJob(ctx, job); err != nil {
		return err
	}

	camp, err := r.campaigns.FindByID(ctx, job.TeamID, job.CampaignID)
	if err != nil {
		return err
	}
	eligible := camp.TotalContacts
	if eligible < 0 {
		eligible = 0
	}
	if err := r.campaigns.UpdateCampaignTotalContacts(ctx, job.CampaignID, eligible); err != nil {
		return err
	}

	job.Status = campaign.JobPlanningBatches
	job.TotalRecipients = count
	job.UpdatedAt = time.Now().UTC()
	if err := r.campaigns.UpdateJob(ctx, job); err != nil {
		return err
	}

	r.log.Info("audience snapshot accepted",
		"campaign_id", job.CampaignID.String(),
		"job_id", job.ID.String(),
		"team_id", job.TeamID.String(),
		"channel_id", job.ChannelID.String(),
		"recipients", count,
		"eligible", eligible,
	)
	return nil
}
