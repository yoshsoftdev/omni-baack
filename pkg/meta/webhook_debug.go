package meta

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WebhookSummary describes a raw Meta webhook payload for logging/debugging.
type WebhookSummary struct {
	Object             string   `json:"object"`
	EntryCount         int      `json:"entry_count"`
	ChangeFields       []string `json:"change_fields"`
	MessageCount       int      `json:"message_count"`
	TextMessageCount   int      `json:"text_message_count"`
	StatusCount        int      `json:"status_count"`
	SkippedMessages    []string `json:"skipped_messages,omitempty"`
	PhoneNumberIDs     []string `json:"phone_number_ids,omitempty"`
}

type webhookStatus struct {
	ID string `json:"id"`
}

func SummarizeWebhookPayload(data []byte) (WebhookSummary, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return WebhookSummary{}, fmt.Errorf("summarize webhook: %w", err)
	}

	summary := WebhookSummary{
		Object:     payload.Object,
		EntryCount: len(payload.Entry),
	}

	phoneIDs := make(map[string]struct{})
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			summary.ChangeFields = append(summary.ChangeFields, change.Field)

			if change.Value.Metadata.PhoneNumberID != "" {
				phoneIDs[change.Value.Metadata.PhoneNumberID] = struct{}{}
			}

			for _, msg := range change.Value.Messages {
				summary.MessageCount++
				switch {
				case msg.Type == "text" && msg.Text.Body != "":
					summary.TextMessageCount++
				default:
					summary.SkippedMessages = append(summary.SkippedMessages,
						fmt.Sprintf("message_id=%s type=%s from=%s", msg.ID, msg.Type, msg.From),
					)
				}
			}

			if len(change.Value.Messages) == 0 {
				var statusPayload struct {
					Statuses []webhookStatus `json:"statuses"`
				}
				if err := json.Unmarshal(mustRemarshal(change.Value), &statusPayload); err == nil {
					summary.StatusCount += len(statusPayload.Statuses)
				}
			}
		}
	}

	for id := range phoneIDs {
		summary.PhoneNumberIDs = append(summary.PhoneNumberIDs, id)
	}
	return summary, nil
}

func mustRemarshal(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func TruncateForLog(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

func HasSignatureHeader(signatureHeader string) bool {
	return strings.TrimSpace(signatureHeader) != ""
}
