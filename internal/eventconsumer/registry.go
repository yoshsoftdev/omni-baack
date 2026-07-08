package eventconsumer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/channelsync"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer/resolver"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// Registry routes canonical Meta events to typed handlers.
type Registry struct {
	legacy       *LegacyConsumer
	messages     *MessageHandler
	statuses     *StatusHandler
	templates    *TemplateHandler
	channels     *ChannelHandler
	dlq          ConsumerDLQ
	metrics      *Metrics
	log          *slog.Logger
}

type Deps struct {
	Channels      channel.Repository
	Contacts      *contact.Service
	Conversations *conversation.Service
	Messages      *message.Service
	MetaMedia     MetaMediaClient
	TokenCipher   TokenCipher
	Storage       StorageUploader
	Templates     template.Repository
	Onboarding    *onboarding.Service
	ChannelSync   *channelsync.Service
	Inbox         Syncer
	Delivery      CampaignDelivery
	Idempotency   idempotency.Store
	DLQ           ConsumerDLQ
	Logger        *slog.Logger
}

func NewRegistry(deps Deps) *Registry {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	metrics := &Metrics{}
	tenantResolver := resolver.NewTenantResolver(deps.Channels)
	store := deps.Idempotency
	if store == nil {
		store = idempotency.NewMemoryStore(0)
	}

	return &Registry{
		legacy:    NewLegacyConsumer(deps.Messages, deps.Onboarding, deps.ChannelSync, log),
		messages:  NewMessageHandler(tenantResolver, deps.Channels, deps.Contacts, deps.Conversations, deps.Messages, deps.Inbox, deps.MetaMedia, deps.TokenCipher, deps.Storage, store, metrics, log),
		statuses:  NewStatusHandler(tenantResolver, deps.Messages, deps.Delivery, store, metrics, log),
		templates: NewTemplateHandler(tenantResolver, deps.Channels, deps.Templates, store, metrics, log),
		channels:  NewChannelHandler(tenantResolver, deps.Channels, deps.ChannelSync, store, metrics, log),
		dlq:       deps.DLQ,
		metrics:   metrics,
		log:       log,
	}
}

func (r *Registry) Metrics() *Metrics {
	return r.metrics
}

// HandleCanonical dispatches a canonical Meta event.
func (r *Registry) HandleCanonical(ctx context.Context, event sharedevents.CanonicalEvent) error {
	if strings.HasPrefix(event.Type, "meta.") || event.Provider == "" {
		// ignore non-canonical subjects accidentally routed here
	}
	r.metrics.IncEvents()

	var err error
	switch event.Type {
	case sharedevents.TypeMessageReceived:
		err = r.messages.Handle(ctx, event)
	case sharedevents.TypeMessageStatusSent, sharedevents.TypeMessageStatusDelivered,
		sharedevents.TypeMessageStatusRead, sharedevents.TypeMessageStatusFailed, sharedevents.TypeMessageDeleted:
		err = r.statuses.Handle(ctx, event)
	case sharedevents.TypeTemplateUpdated:
		err = r.templates.Handle(ctx, event)
	case sharedevents.TypeChannelUpdated:
		err = r.channels.Handle(ctx, event)
	default:
		r.log.Warn("unknown canonical event type", "type", event.Type, "event_id", event.EventID)
		return nil
	}

	if err == nil {
		return nil
	}
	if te, ok := IsTerminal(err); ok {
		category := te.Category
		if category == "" {
			category = sharedevents.ConsumerDLQCategory(event.Type)
		}
		r.metrics.IncDLQ()
		if r.dlq != nil {
			if dlqErr := r.dlq.PublishConsumerDLQ(ctx, category, te.Error(), event); dlqErr != nil {
				r.log.Error("consumer dlq publish failed", "error", dlqErr, "event_id", event.EventID)
				return err
			}
		}
		r.log.Warn("event routed to consumer dlq",
			"category", category,
			"event_id", event.EventID,
			"event_type", event.Type,
			"error", te.Error(),
		)
		return nil
	}
	r.metrics.IncFailures()
	return err
}

// HandleLegacy dispatches legacy events.> envelope events.
func (r *Registry) HandleLegacy(ctx context.Context, envelope sharedevents.Envelope) error {
	return r.legacy.Handle(ctx, envelope)
}
