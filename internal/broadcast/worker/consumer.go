package worker

import (
	"context"
	"log/slog"

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
	_, err = js.AddConsumer(c.stream, &nats.ConsumerConfig{
		Durable:       c.durable,
		FilterSubject: sharedevents.SubjectBroadcastBatchSend,
		AckPolicy:     nats.AckExplicitPolicy,
	})
	if err != nil && err != nats.ErrConsumerNameAlreadyInUse {
		c.log.Warn("add consumer", "error", err)
	}
	_, err = js.Subscribe(sharedevents.SubjectBroadcastBatchSend, func(msg *nats.Msg) {
		if err := c.processor.HandleRaw(ctx, msg.Data); err != nil {
			c.log.Error("batch send failed", "error", err)
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	}, nats.Bind(c.stream, c.durable))
	return err
}
