package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// Publisher sends broadcast NATS messages.
type Publisher interface {
	PublishJSON(ctx context.Context, subject string, payload any) error
}

// OutboxStore persists reliable publish intents.
type OutboxStore interface {
	InsertOutbox(ctx context.Context, subject string, payload []byte) error
	ListPendingOutbox(ctx context.Context, limit int) ([]campaign.OutboxMessage, error)
	MarkOutboxPublished(ctx context.Context, id uuid.UUID) error
}

// OutboxRelay drains pending outbox rows to NATS.
type OutboxRelay struct {
	store     OutboxStore
	publisher Publisher
	log       *slog.Logger
}

func NewOutboxRelay(store OutboxStore, publisher Publisher, log *slog.Logger) *OutboxRelay {
	if log == nil {
		log = slog.Default()
	}
	return &OutboxRelay{store: store, publisher: publisher, log: log}
}

func (r *OutboxRelay) RunOnce(ctx context.Context) error {
	msgs, err := r.store.ListPendingOutbox(ctx, 100)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		var payload any
		_ = json.Unmarshal(m.Payload, &payload)
		if err := r.publisher.PublishJSON(ctx, m.Subject, payload); err != nil {
			r.log.Error("outbox publish failed", "outbox_id", m.ID.String(), "subject", m.Subject, "error", err)
			continue
		}
		if err := r.store.MarkOutboxPublished(ctx, m.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *OutboxRelay) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultOutboxPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil {
			r.log.Error("outbox relay failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// EnqueueJobCreated stores a job.created event in the outbox.
func EnqueueJobCreated(ctx context.Context, store OutboxStore, job *campaign.BroadcastJob) error {
	payload, err := json.Marshal(sharedevents.JobCreatedPayload{
		JobID:      job.ID.String(),
		CampaignID: job.CampaignID.String(),
		TeamID:     job.TeamID.String(),
		ChannelID:  job.ChannelID.String(),
	})
	if err != nil {
		return err
	}
	return store.InsertOutbox(ctx, sharedevents.SubjectBroadcastJobCreated, payload)
}

// PublishBatchSend enqueues a batch.send event for reliable delivery.
func PublishBatchSend(ctx context.Context, store OutboxStore, _ Publisher, batch *campaign.BroadcastBatch, job *campaign.BroadcastJob) error {
	payload := sharedevents.BatchSendPayload{
		JobID:          job.ID.String(),
		BatchID:        batch.ID.String(),
		CampaignID:     batch.CampaignID.String(),
		TeamID:         job.TeamID.String(),
		ChannelID:      job.ChannelID.String(),
		RecipientCount: batch.RecipientCount,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return store.InsertOutbox(ctx, sharedevents.SubjectBroadcastBatchSend, raw)
}
