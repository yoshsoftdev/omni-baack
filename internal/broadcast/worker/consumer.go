package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
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
				continue
			}
			for _, msg := range msgs {
				if err := c.processor.HandleRaw(ctx, msg.Data); err != nil {
					c.log.Error("batch send failed", "error", err)
					_ = msg.Nak()
					continue
				}
				_ = msg.Ack()
			}
		}
	}()
	return nil
}
