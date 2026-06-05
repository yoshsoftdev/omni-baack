package meta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type WebhookPayload struct {
	Object string         `json:"object"`
	Entry  []WebhookEntry `json:"entry"`
}

type WebhookEntry struct {
	Changes []WebhookChange `json:"changes"`
}

type WebhookChange struct {
	Field string        `json:"field"`
	Value WebhookValue  `json:"value"`
}

type WebhookValue struct {
	Metadata WebhookMetadata `json:"metadata"`
	Messages []WebhookMessage `json:"messages"`
}

type WebhookMetadata struct {
	PhoneNumberID string `json:"phone_number_id"`
}

type WebhookMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
}

type InboundMessage struct {
	PhoneNumberID string
	From          string
	WAMID         string
	Content       string
}

func ParseWebhookPayload(data []byte) ([]InboundMessage, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}

	var messages []InboundMessage
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			phoneNumberID := change.Value.Metadata.PhoneNumberID
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" || msg.Text.Body == "" {
					continue
				}
				messages = append(messages, InboundMessage{
					PhoneNumberID: phoneNumberID,
					From:          msg.From,
					WAMID:         msg.ID,
					Content:       msg.Text.Body,
				})
			}
		}
	}
	return messages, nil
}

func VerifySignature(appSecret, signatureHeader string, body []byte) bool {
	if appSecret == "" || signatureHeader == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	expectedMAC := strings.TrimPrefix(signatureHeader, prefix)

	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	computed := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computed), []byte(expectedMAC))
}
