package automation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

// Forwarder pushes inbound messages to customer-registered webhook URLs.
type Forwarder struct {
	repo       Repository
	httpClient *http.Client
}

func NewForwarder(repo Repository) *Forwarder {
	return &Forwarder{
		repo:       repo,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type ForwardInboundInput struct {
	TeamID         uuid.UUID
	ChannelID      uuid.UUID
	ChannelType    string
	ConversationID uuid.UUID
	Contact        *contact.Contact
	Message        *message.Message
}

// ForwardInbound delivers message.received to all matching active webhooks.
// Failures are logged and never bubble up to the inbound pipeline.
func (f *Forwarder) ForwardInbound(ctx context.Context, input ForwardInboundInput) {
	if f == nil || f.repo == nil || input.Message == nil || input.Message.ID == uuid.Nil {
		return
	}
	webhooks, err := f.repo.ListActiveForwardWebhooks(ctx, input.TeamID)
	if err != nil || len(webhooks) == 0 {
		return
	}

	payload := InboundForwardPayload{
		Event:          EventMessageReceived,
		OccurredAt:     input.Message.CreatedAt,
		TeamID:         input.TeamID.String(),
		ChannelID:      input.ChannelID.String(),
		ChannelType:    input.ChannelType,
		ConversationID: input.ConversationID.String(),
		Message: InboundForwardMessage{
			ID:          input.Message.ID.String(),
			Direction:   string(input.Message.Direction),
			ContentType: string(input.Message.ContentType),
			Content:     input.Message.Content,
			CreatedAt:   input.Message.CreatedAt,
		},
	}
	if input.Message.WAMID != nil {
		payload.Message.WAMID = *input.Message.WAMID
	}
	if input.Contact != nil {
		payload.Contact = InboundForwardContact{
			ID:           input.Contact.ID.String(),
			Phone:        input.Contact.Phone,
			ContactName:  input.Contact.ContactName,
			CustomerName: input.Contact.CustomerName,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	for i := range webhooks {
		wh := webhooks[i]
		if !webhookSubscribes(wh.Events, EventMessageReceived) {
			continue
		}
		if wh.ChannelID != nil && *wh.ChannelID != input.ChannelID {
			continue
		}
		f.deliver(ctx, &wh, body, input)
	}
}

func webhookSubscribes(events []string, event string) bool {
	if len(events) == 0 {
		return event == EventMessageReceived
	}
	for _, item := range events {
		if strings.TrimSpace(item) == event {
			return true
		}
	}
	return false
}

func (f *Forwarder) deliver(ctx context.Context, wh *ForwardWebhook, body []byte, input ForwardInboundInput) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		f.record(ctx, wh, input, 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Omni-Event", EventMessageReceived)
	req.Header.Set("X-Omni-Webhook-Id", wh.ID)
	if secret := strings.TrimSpace(wh.Secret); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Omni-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	client := f.httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		f.record(ctx, wh, input, 0, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		f.record(ctx, wh, input, resp.StatusCode, "")
		_ = f.repo.TouchForwardWebhookUsed(ctx, wh.ID, time.Now().UTC())
		return
	}
	f.record(ctx, wh, input, resp.StatusCode, "non-2xx webhook response")
}

func (f *Forwarder) record(ctx context.Context, wh *ForwardWebhook, input ForwardInboundInput, statusCode int, errMsg string) {
	status := ForwardDeliverySuccess
	var errPtr *string
	if strings.TrimSpace(errMsg) != "" || statusCode >= 400 || statusCode == 0 {
		status = ForwardDeliveryFailed
		msg := strings.TrimSpace(errMsg)
		if msg == "" {
			msg = "webhook delivery failed"
		}
		errPtr = &msg
	}
	msgID := input.Message.ID
	convID := input.ConversationID
	_ = f.repo.CreateForwardDelivery(ctx, &ForwardDelivery{
		ID:             NewForwardDeliveryID(),
		TeamID:         wh.TeamID,
		WebhookID:      wh.ID,
		EventType:      EventMessageReceived,
		MessageID:      &msgID,
		ConversationID: &convID,
		Status:         status,
		StatusCode:     statusCode,
		ErrorMessage:   errPtr,
		AttemptedAt:    time.Now().UTC(),
	})
}
