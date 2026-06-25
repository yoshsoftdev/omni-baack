package campaign

import (
	"context"
	"encoding/json"

	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// OutboxInserter persists broadcast NATS publish intents.
type OutboxInserter interface {
	InsertOutbox(ctx context.Context, subject string, payload []byte) error
}

// EnqueueBroadcastJobCreated stores a broadcast.job.created event in the outbox.
func EnqueueBroadcastJobCreated(ctx context.Context, store OutboxInserter, job *BroadcastJob) error {
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
