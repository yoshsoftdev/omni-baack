package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	sharedconfig "github.com/omnichannel/omni-chan-v2/shared/config"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
	sharednats "github.com/omnichannel/omni-chan-v2/shared/nats"
)

// Consumer subscribes to broadcast.batch.send events.
type Consumer struct {
	processor *BatchProcessor
	stream    string
	durable   string
	log       *slog.Logger
}

func NewConsumer(processor *BatchProcessor, stream, durable string, log *slog.Logger) *Consumer {
	if log == nil {
		log = slog.Default()
	}
	if durable == "" {
		durable = "broadcast-send-worker"
	}
	if stream == "" {
		stream = "BROADCAST"
	}
	return &Consumer{processor: processor, stream: stream, durable: durable, log: log}
}

func (c *Consumer) Subscribe(ctx context.Context, nc *nats.Conn) error {
	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	// PullSubscribe matches JetStream durable pull consumers (default when no DeliverSubject).
	sub, err := js.PullSubscribe(
		sharedevents.SubjectBroadcastBatchSend,
		c.durable,
		nats.BindStream(c.stream),
		nats.ManualAck(),
	)
	if err != nil {
		return err
	}
	c.log.Info("broadcast batch.send subscriber started", "stream", c.stream, "consumer", c.durable)
	redelivery := sharednats.RedeliveryPolicyFrom(sharedconfig.NATS{})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msgs, err := sub.Fetch(10, nats.MaxWait(2*time.Second))
			if err != nil {
				if err == nats.ErrTimeout || err == context.Canceled || err == context.DeadlineExceeded {
					continue
				}
				c.log.Error("batch.send fetch failed", "error", err)
				sharednats.PauseAfterFetchError(ctx)
				continue
			}
			for _, msg := range msgs {
				msgCtx, span := sharednats.ConsumerSpan(ctx, msg)
				err := c.processor.HandleRaw(msgCtx, msg.Data)
				span.End()
				if err != nil {
					// Backoff, never terminate: worker recovery requeues stale batches.
					attempt := redelivery.NakWithBackoff(msg)
					c.log.Error("batch send failed", "attempt", attempt, "error", err)
					continue
				}
				_ = msg.Ack()
			}
		}
	}()
	return nil
}
