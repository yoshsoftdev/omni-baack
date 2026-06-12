package eventconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

type Consumer struct {
	messages   *message.Service
	onboarding *onboarding.Service
	log        *slog.Logger
}

func New(messages *message.Service, onboarding *onboarding.Service, log *slog.Logger) *Consumer {
	return &Consumer{messages: messages, onboarding: onboarding, log: log}
}

func (c *Consumer) Handle(ctx context.Context, envelope sharedevents.Envelope) error {
	switch envelope.Type {
	case sharedevents.WhatsAppMessageReceived:
		var payload sharedevents.WhatsAppMessageReceivedPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf("decode whatsapp message: %w", err)
		}
		return c.messages.HandleMetaInbound(ctx, metaclient.InboundMessage{
			PhoneNumberID: payload.PhoneNumberID,
			From:          payload.From,
			WAMID:         payload.WAMID,
			Content:       payload.Content,
		})
	case sharedevents.WhatsAppAccountUpdate:
		var payload sharedevents.WhatsAppAccountUpdatePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf("decode account update: %w", err)
		}
		return c.handleAccountUpdate(ctx, payload)
	case sharedevents.WhatsAppMessageStatus:
		c.log.Info("whatsapp message status", "payload", string(envelope.Payload))
		return nil
	case sharedevents.WhatsAppTemplateStatus:
		c.log.Info("whatsapp template status", "payload", string(envelope.Payload))
		return nil
	case sharedevents.TelegramMessageReceived, sharedevents.TelegramCallbackReceived:
		c.log.Info("telegram event received", "type", envelope.Type)
		return nil
	default:
		c.log.Warn("unknown event type", "type", envelope.Type)
		return nil
	}
}

func (c *Consumer) handleAccountUpdate(ctx context.Context, payload sharedevents.WhatsAppAccountUpdatePayload) error {
	if c.onboarding == nil || len(payload.Raw) == 0 {
		return nil
	}
	wrapped := fmt.Sprintf(
		`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":%q,"value":%s}]}]}`,
		payload.Field,
		string(payload.Raw),
	)
	events, err := metaclient.ParseCoexWebhookPayload([]byte(wrapped))
	if err != nil {
		return fmt.Errorf("parse account update: %w", err)
	}
	return c.onboarding.HandleWebhook(ctx, events)
}
