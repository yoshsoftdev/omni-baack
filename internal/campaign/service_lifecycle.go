package campaign

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// SendResult is returned after a campaign is queued for broadcast.
type SendResult struct {
	Campaign *Campaign
	Job      *BroadcastJob
}

// Send queues a draft or scheduled campaign for orchestrated delivery.
func (s *Service) Send(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID) (*SendResult, error) {
	if err := s.authorize(actorRole, rbac.PermCampaignSend); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	camp, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if !CanSendCampaign(camp.Status) {
		return nil, serviceerrors.NewValidation("status", "campaign cannot be sent in status "+string(camp.Status))
	}
	if camp.Status == StatusDraft && camp.SendMode != nil && *camp.SendMode == SendLater {
		return nil, serviceerrors.NewValidation("send_mode", "campaign is scheduled for later; wait for scheduler or update schedule")
	}

	if s.payment != nil {
		configured, err := s.payment.ChannelPaymentConfigured(ctx, teamID, camp.ChannelID)
		if err != nil {
			return nil, err
		}
		if !configured {
			return nil, serviceerrors.NewPaymentRequired("")
		}
	}

	if err := s.campaigns.TransitionCampaignToQueuedForSend(ctx, teamID, campaignID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	camp.Status = StatusQueued

	job, err := s.campaigns.FindActiveJobByCampaign(ctx, campaignID)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, serviceerrors.MapRepository(err)
		}
		now := time.Now().UTC()
		job = &BroadcastJob{
			ID:         uuid.New(),
			CampaignID: campaignID,
			TeamID:     teamID,
			ChannelID:  camp.ChannelID,
			Status:     JobPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := s.campaigns.CreateJob(ctx, job); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
	}

	orch, ok := s.campaigns.(OutboxInserter)
	if !ok {
		return nil, serviceerrors.NewValidation("orchestration", "broadcast dispatch is not configured")
	}
	if err := EnqueueBroadcastJobCreated(ctx, orch, job); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	return &SendResult{Campaign: camp, Job: job}, nil
}

// Cancel stops a campaign and its non-terminal recipients per Phase 8 rules.
func (s *Service) Cancel(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID, reason string) (*Campaign, error) {
	if err := s.authorize(actorRole, rbac.PermCampaignCancel); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	camp, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if camp.Status == StatusCancelled {
		return camp, nil
	}
	if !CanCancelCampaign(camp.Status) {
		return nil, serviceerrors.ErrConflict
	}

	if err := s.campaigns.CancelCampaignExecution(ctx, teamID, campaignID, reason); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.cancelFlags != nil {
		_ = s.cancelFlags.Set(ctx, campaignID)
	}
	camp.Status = StatusCancelled
	return camp, nil
}
