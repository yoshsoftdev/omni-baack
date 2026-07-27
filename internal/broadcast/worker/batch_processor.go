package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// CampaignLifecycle finalizes campaign status when all recipients are terminal.
type CampaignLifecycle interface {
	Check(ctx context.Context, teamID, campaignID uuid.UUID, at time.Time) (bool, error)
}

// BatchProcessor executes send work for a broadcast batch.
type BatchProcessor struct {
	cfg           Config
	sends         campaign.SendRepository
	campaigns     CampaignStore
	templates     TemplateStore
	channels      ChannelStore
	sender        *Sender
	finalizer     SendFinalizer
	cancelChecker CampaignCancelChecker
	lifecycle     CampaignLifecycle
	inbox         InboxRecorder
	limiter       RateLimiter
	metrics       *Metrics
	log           *slog.Logger
}

func NewBatchProcessor(
	cfg Config,
	sends campaign.SendRepository,
	campaigns CampaignStore,
	templates TemplateStore,
	channels ChannelStore,
	sender *Sender,
	finalizer SendFinalizer,
	cancelChecker CampaignCancelChecker,
	limiter RateLimiter,
	metrics *Metrics,
	log *slog.Logger,
) *BatchProcessor {
	if log == nil {
		log = slog.Default()
	}
	if limiter == nil {
		limiter = NewMemoryRateLimiter(cfg.RateLimitPerSec)
	}
	if metrics == nil {
		metrics = NewMetrics()
	}
	return &BatchProcessor{
		cfg:           cfg.Normalize(),
		sends:         sends,
		campaigns:     campaigns,
		templates:     templates,
		channels:      channels,
		sender:        sender,
		finalizer:     finalizer,
		cancelChecker: cancelChecker,
		limiter:       limiter,
		metrics:       metrics,
		log:           log,
	}
}

// WithLifecycle wires campaign completion checks after batches finish.
func (p *BatchProcessor) WithLifecycle(lifecycle CampaignLifecycle) *BatchProcessor {
	if p == nil {
		return nil
	}
	p.lifecycle = lifecycle
	return p
}

// WithInboxRecorder mirrors successful Meta sends into Inbox conversation history.
func (p *BatchProcessor) WithInboxRecorder(recorder InboxRecorder) *BatchProcessor {
	if p == nil {
		return nil
	}
	p.inbox = recorder
	return p
}

func (p *BatchProcessor) HandleBatchSend(ctx context.Context, payload sharedevents.BatchSendPayload) error {
	start := time.Now()
	defer func() { p.metrics.ObserveBatchProcessing(time.Since(start)) }()

	batchID, err := uuid.Parse(payload.BatchID)
	if err != nil {
		return err
	}
	jobID, err := uuid.Parse(payload.JobID)
	if err != nil {
		return err
	}
	campaignID, err := uuid.Parse(payload.CampaignID)
	if err != nil {
		return err
	}
	teamID, err := uuid.Parse(payload.TeamID)
	if err != nil {
		return err
	}
	channelID, err := uuid.Parse(payload.ChannelID)
	if err != nil {
		return err
	}

	existing, err := p.sends.FindBatchByID(ctx, batchID)
	if err != nil {
		return err
	}
	if existing.Status == campaign.BatchCompleted {
		return nil
	}

	staleBefore := time.Now().UTC().Add(-p.cfg.BatchLease)
	batch, err := p.sends.ClaimBatchForProcessing(ctx, batchID, p.cfg.WorkerID, staleBefore)
	if err != nil {
		if errors.Is(err, campaign.ErrBatchInProgress) {
			return nil
		}
		return err
	}
	if batch.Status == campaign.BatchCompleted {
		return nil
	}
	p.metrics.IncWorkerBatchesProcessing()

	_ = p.sends.TransitionCampaignToSending(ctx, teamID, campaignID)

	camp, err := p.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return err
	}
	tpl, err := p.templates.FindByID(ctx, teamID, camp.TemplateID)
	if err != nil {
		return err
	}
	ch, err := p.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return err
	}

	recipients, err := p.sends.ListRecipientsByBatch(ctx, batchID)
	if err != nil {
		return err
	}

	phoneNumberID := PhoneNumberID(ch)
	for i := range recipients {
		rec := &recipients[i]
		if skipRecipient(rec) {
			continue
		}
		if rec.Status != campaign.RecipientQueued {
			continue
		}
		if p.isCampaignCancelled(ctx, campaignID) {
			if err := p.skipRecipientForCancel(ctx, rec); err != nil {
				return err
			}
			continue
		}

		for {
			if err := p.limiter.Acquire(ctx, channelID); err != nil {
				if errors.Is(err, ErrRateLimited) {
					p.metrics.IncMetaRateLimit(channelID)
					time.Sleep(p.cfg.RateLimitBackoff)
					continue
				}
				return err
			}
			break
		}

		if err := p.processRecipient(ctx, camp, tpl, ch, batch, jobID, phoneNumberID, rec); err != nil {
			return err
		}

		if (i+1)%10 == 0 {
			_ = p.sends.RefreshBatchLease(ctx, batchID)
		}
	}

	active, err := p.sends.BatchHasActiveRecipients(ctx, batchID)
	if err != nil {
		return err
	}
	if active {
		_ = p.sends.RefreshBatchLease(ctx, batchID)
		return nil
	}

	if err := p.sends.CompleteBatch(ctx, batchID); err != nil {
		return err
	}
	p.metrics.IncWorkerBatchesCompleted()
	if p.lifecycle != nil {
		if _, checkErr := p.lifecycle.Check(ctx, teamID, campaignID, time.Now().UTC()); checkErr != nil {
			p.log.Warn("campaign completion check failed",
				"campaign_id", campaignID.String(),
				"team_id", teamID.String(),
				"error", checkErr.Error(),
			)
		}
	}
	return nil
}

func (p *BatchProcessor) processRecipient(
	ctx context.Context,
	camp *campaign.Campaign,
	tpl *template.Template,
	ch *channel.Channel,
	batch *campaign.BroadcastBatch,
	jobID uuid.UUID,
	phoneNumberID string,
	rec *campaign.Recipient,
) error {
	log := p.log.With(
		"campaign_id", camp.ID.String(),
		"job_id", jobID.String(),
		"batch_id", batch.ID.String(),
		"recipient_id", rec.ID.String(),
		"channel_id", ch.ID.String(),
		"phone_number_id", phoneNumberID,
	)

	if err := p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, campaign.RecipientQueued, campaign.RecipientSending, campaign.RecipientDeliveryUpdate{}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	rec.Status = campaign.RecipientSending

	if p.isCampaignCancelled(ctx, camp.ID) {
		return p.skipRecipientForCancel(ctx, rec)
	}

	for attempt := 0; attempt < p.cfg.MaxSendAttempts; attempt++ {
		p.metrics.IncSendAttempt(ch.ID)
		result, sendErr := p.sender.Send(ctx, SendInput{
			Campaign:  camp,
			Template:  tpl,
			Channel:   ch,
			Recipient: rec,
		})
		if sendErr == nil {
			wamid := result.WAMID
			now := time.Now().UTC()
			applied := true
			var err error
			if p.finalizer != nil {
				applied, err = p.finalizer.FinalizeRecipientSent(ctx, campaign.FinalizeSentInput{
					RecipientID:      rec.ID,
					CampaignID:       rec.CampaignID,
					TeamID:           rec.TeamID,
					From:             campaign.RecipientSending,
					WAMID:            wamid,
					ProviderResponse: result.ProviderResponse,
					SentAt:           now,
				})
			} else {
				err = p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, campaign.RecipientSending, campaign.RecipientSent, campaign.RecipientDeliveryUpdate{
					WAMID:            &wamid,
					ProviderResponse: result.ProviderResponse,
				})
			}
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return nil
				}
				return err
			}
			if applied {
				_ = p.sends.IncrementBatchRecipientSent(ctx, batch.ID)
				p.metrics.IncSendSuccess(ch.ID)
			}
			rec.Status = campaign.RecipientSent
			rec.WAMID = &wamid
			p.mirrorToInbox(ctx, camp, tpl, ch, rec, wamid, result.ProviderResponse, now, log)
			log.Info("recipient sent")
			return nil
		}

		kind, code, reason := ClassifySendError(sendErr)
		switch kind {
		case FailureRateLimited:
			p.metrics.IncMetaRateLimit(ch.ID)
			time.Sleep(p.cfg.RateLimitBackoff)
			continue
		case FailurePermanent:
			msg := reason
			if err := p.markFailed(ctx, rec, batch, code, msg); err != nil {
				return err
			}
			p.metrics.IncSendFailed(ch.ID)
			log.Warn("recipient failed permanently", "failure_code", code, "error", msg)
			return nil
		}

		if attempt+1 >= p.cfg.MaxSendAttempts {
			msg := reason
			if err := p.markFailed(ctx, rec, batch, code, msg); err != nil {
				return err
			}
			p.metrics.IncSendFailed(ch.ID)
			log.Warn("recipient failed after retries", "failure_code", code, "error", msg)
			return nil
		}

		p.metrics.IncSendRetry(ch.ID)
		errMsg := reason
		if err := p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, campaign.RecipientSending, campaign.RecipientQueued, campaign.RecipientDeliveryUpdate{
			LastError:        &errMsg,
			FailureCode:      &code,
			IncrementAttempt: true,
		}); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil
			}
			return err
		}
		rec.Status = campaign.RecipientQueued
		rec.AttemptCount++

		delay := p.cfg.delayForAttempt(rec.AttemptCount)
		log.Warn("recipient send retry", "attempt", rec.AttemptCount, "delay", delay.String(), "error", reason)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		if err := p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, campaign.RecipientQueued, campaign.RecipientSending, campaign.RecipientDeliveryUpdate{}); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil
			}
			return err
		}
		rec.Status = campaign.RecipientSending
	}

	return nil
}

func (p *BatchProcessor) isCampaignCancelled(ctx context.Context, campaignID uuid.UUID) bool {
	if p.cancelChecker == nil {
		return false
	}
	cancelled, err := p.cancelChecker.IsSet(ctx, campaignID)
	return err == nil && cancelled
}

func (p *BatchProcessor) skipRecipientForCancel(ctx context.Context, rec *campaign.Recipient) error {
	if skipRecipient(rec) {
		return nil
	}
	from := rec.Status
	switch from {
	case campaign.RecipientQueued, campaign.RecipientPending, campaign.RecipientSending:
		if from == campaign.RecipientSending && rec.WAMID != nil && *rec.WAMID != "" {
			return nil
		}
		err := p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, from, campaign.RecipientCancelled, campaign.RecipientDeliveryUpdate{})
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	default:
		return nil
	}
}

func (p *BatchProcessor) markFailed(
	ctx context.Context,
	rec *campaign.Recipient,
	batch *campaign.BroadcastBatch,
	code, msg string,
) error {
	from := rec.Status
	if from != campaign.RecipientSending && from != campaign.RecipientQueued {
		from = campaign.RecipientSending
	}
	if p.finalizer != nil {
		applied, err := p.finalizer.FinalizeRecipientFailed(ctx, campaign.FinalizeFailedInput{
			RecipientID: rec.ID,
			CampaignID:  rec.CampaignID,
			TeamID:      rec.TeamID,
			From:        from,
			FailureCode: code,
			LastError:   msg,
		})
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil
			}
			return err
		}
		if applied {
			_ = p.sends.IncrementBatchRecipientFailed(ctx, batch.ID)
		}
		return nil
	}

	errMsg := msg
	if err := p.campaigns.UpdateRecipientDelivery(ctx, rec.ID, from, campaign.RecipientFailed, campaign.RecipientDeliveryUpdate{
		LastError:   &errMsg,
		FailureCode: &code,
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	_ = p.sends.IncrementBatchRecipientFailed(ctx, batch.ID)
	return nil
}

func (p *BatchProcessor) mirrorToInbox(
	ctx context.Context,
	camp *campaign.Campaign,
	tpl *template.Template,
	ch *channel.Channel,
	rec *campaign.Recipient,
	wamid string,
	providerResponse json.RawMessage,
	sentAt time.Time,
	log *slog.Logger,
) {
	if p.inbox == nil {
		return
	}
	variables, err := campaign.ParseRecipientVariables(rec.Variables)
	if err != nil {
		log.Warn("inbox mirror skipped: invalid recipient variables", "error", err)
		return
	}
	if err := p.inbox.RecordCampaignSend(ctx, CampaignInboxRecordInput{
		TeamID:           camp.TeamID,
		ChannelID:        ch.ID,
		ContactID:        rec.ContactID,
		CampaignName:     camp.Name,
		Template:         tpl,
		RecipientPhone:   rec.PhoneNumber,
		Variables:        variables,
		WAMID:            wamid,
		ProviderResponse: providerResponse,
		SentAt:           sentAt,
	}); err != nil {
		// Campaign send already succeeded at Meta; do not fail the recipient.
		log.Warn("inbox mirror failed", "error", err, "wamid", wamid)
	}
}

func skipRecipient(rec *campaign.Recipient) bool {
	if rec.WAMID != nil && *rec.WAMID != "" {
		return true
	}
	switch rec.Status {
	case campaign.RecipientSent, campaign.RecipientDelivered, campaign.RecipientRead:
		return true
	default:
		return false
	}
}

// HandleRaw decodes a NATS payload and processes the batch.
func (p *BatchProcessor) HandleRaw(ctx context.Context, data []byte) error {
	var payload sharedevents.BatchSendPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return p.HandleBatchSend(ctx, payload)
}
