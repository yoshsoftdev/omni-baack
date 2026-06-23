package campaign

import (
	"context"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/campaignqueue"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// Worker is a no-op placeholder until the broadcast job worker is migrated.
type Worker struct{}

// NewWorker returns a worker stub that satisfies bootstrap wiring.
func NewWorker(
	_ Repository,
	_ contact.Repository,
	_ device.Repository,
	_ template.Repository,
	_ campaignqueue.Queue,
	_ *metaclient.Client,
	_ any,
	_ *logger.Logger,
) *Worker {
	return &Worker{}
}

// Start blocks until the context is cancelled.
func (w *Worker) Start(ctx context.Context) {
	<-ctx.Done()
}
