package worker

import (
	"context"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
)

// Worker runs the broadcast send consumer.
type Worker struct {
	consumer *Consumer
	nc       *nats.Conn
	log      *slog.Logger
}

func NewWorker(consumer *Consumer, nc *nats.Conn, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{consumer: consumer, nc: nc, log: log}
}

func (w *Worker) Start(ctx context.Context) error {
	if err := w.consumer.Subscribe(ctx, w.nc); err != nil {
		return err
	}
	w.log.Info("broadcast send worker started")
	<-ctx.Done()
	w.log.Info("broadcast send worker shutting down")
	return nil
}

// ResolveWorkerID returns a stable worker identity for batch leases.
func ResolveWorkerID(configured string) string {
	if configured != "" {
		return configured
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "broadcast-worker"
	}
	return host
}
