package meta

import (
	"encoding/json"
	"fmt"
)

type CoexWebhookEvents struct {
	InboundMessages   []InboundMessage
	ContactSyncEvents []ContactSyncEvent
	HistoryEvents     []HistoryEvent
	MessageEchoes     []MessageEchoEvent
}

type ContactSyncEvent struct {
	PhoneNumberID string
	Action        string
	FullName      string
	FirstName     string
	PhoneNumber   string
}

type HistoryEvent struct {
	PhoneNumberID string
	Phase         int
	ChunkOrder    int
	Progress      int
	Threads       []HistoryThread
	Declined      bool
	DeclineReason string
}

type HistoryThread struct {
	ContactPhone string
	Messages     []HistoryMessage
}

type HistoryMessage struct {
	From      string
	To        string
	ID        string
	Timestamp string
	Type      string
	Body      string
}

type MessageEchoEvent struct {
	PhoneNumberID string
	From          string
	To            string
	ID            string
	Timestamp     string
	Type          string
	Body          string
}

type coexWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string          `json:"field"`
			Value json.RawMessage `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type coexValueBase struct {
	MessagingProduct string `json:"messaging_product"`
	Metadata         struct {
		PhoneNumberID      string `json:"phone_number_id"`
		DisplayPhoneNumber string `json:"display_phone_number"`
	} `json:"metadata"`
}

func ParseCoexWebhookPayload(data []byte) (*CoexWebhookEvents, error) {
	var payload coexWebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse coex webhook: %w", err)
	}

	events := &CoexWebhookEvents{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			switch change.Field {
			case "messages":
				msgs, err := parseInboundMessages(change.Value)
				if err != nil {
					return nil, err
				}
				events.InboundMessages = append(events.InboundMessages, msgs...)
			case "smb_app_state_sync":
				items, err := parseContactSync(change.Value)
				if err != nil {
					return nil, err
				}
				events.ContactSyncEvents = append(events.ContactSyncEvents, items...)
			case "history":
				items, err := parseHistory(change.Value)
				if err != nil {
					return nil, err
				}
				events.HistoryEvents = append(events.HistoryEvents, items...)
			case "smb_message_echoes":
				items, err := parseMessageEchoes(change.Value)
				if err != nil {
					return nil, err
				}
				events.MessageEchoes = append(events.MessageEchoes, items...)
			}
		}
	}
	return events, nil
}

func parseInboundMessages(raw json.RawMessage) ([]InboundMessage, error) {
	var value struct {
		coexValueBase
		Messages []WebhookMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var messages []InboundMessage
	for _, msg := range value.Messages {
		if msg.Type != "text" || msg.Text.Body == "" {
			continue
		}
		messages = append(messages, InboundMessage{
			PhoneNumberID: value.Metadata.PhoneNumberID,
			From:          msg.From,
			WAMID:         msg.ID,
			Content:       msg.Text.Body,
		})
	}
	return messages, nil
}

func parseContactSync(raw json.RawMessage) ([]ContactSyncEvent, error) {
	var value struct {
		coexValueBase
		StateSync []struct {
			Type    string `json:"type"`
			Action  string `json:"action"`
			Contact struct {
				FullName    string `json:"full_name"`
				FirstName   string `json:"first_name"`
				PhoneNumber string `json:"phone_number"`
			} `json:"contact"`
		} `json:"state_sync"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := make([]ContactSyncEvent, 0, len(value.StateSync))
	for _, sync := range value.StateSync {
		if sync.Type != "contact" {
			continue
		}
		items = append(items, ContactSyncEvent{
			PhoneNumberID: value.Metadata.PhoneNumberID,
			Action:        sync.Action,
			FullName:      sync.Contact.FullName,
			FirstName:     sync.Contact.FirstName,
			PhoneNumber:   sync.Contact.PhoneNumber,
		})
	}
	return items, nil
}

func parseHistory(raw json.RawMessage) ([]HistoryEvent, error) {
	var value struct {
		coexValueBase
		History []json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}

	items := make([]HistoryEvent, 0, len(value.History))
	for _, chunk := range value.History {
		var declined struct {
			Errors []struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(chunk, &declined); err == nil && len(declined.Errors) > 0 {
			reason := declined.Errors[0].Message
			items = append(items, HistoryEvent{
				PhoneNumberID: value.Metadata.PhoneNumberID,
				Declined:      true,
				DeclineReason: reason,
			})
			continue
		}

		var historyChunk struct {
			Metadata struct {
				Phase      int `json:"phase"`
				ChunkOrder int `json:"chunk_order"`
				Progress   int `json:"progress"`
			} `json:"metadata"`
			Threads []struct {
				ID       string `json:"id"`
				Messages []struct {
					From      string `json:"from"`
					To        string `json:"to"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"threads"`
		}
		if err := json.Unmarshal(chunk, &historyChunk); err != nil {
			return nil, err
		}

		event := HistoryEvent{
			PhoneNumberID: value.Metadata.PhoneNumberID,
			Phase:         historyChunk.Metadata.Phase,
			ChunkOrder:    historyChunk.Metadata.ChunkOrder,
			Progress:      historyChunk.Metadata.Progress,
		}
		for _, thread := range historyChunk.Threads {
			hThread := HistoryThread{ContactPhone: thread.ID}
			for _, msg := range thread.Messages {
				body := msg.Text.Body
				if body == "" {
					body = fmt.Sprintf("[%s message]", msg.Type)
				}
				hThread.Messages = append(hThread.Messages, HistoryMessage{
					From:      msg.From,
					To:        msg.To,
					ID:        msg.ID,
					Timestamp: msg.Timestamp,
					Type:      msg.Type,
					Body:      body,
				})
			}
			event.Threads = append(event.Threads, hThread)
		}
		items = append(items, event)
	}
	return items, nil
}

func parseMessageEchoes(raw json.RawMessage) ([]MessageEchoEvent, error) {
	var value struct {
		coexValueBase
		MessageEchoes []struct {
			From      string `json:"from"`
			To        string `json:"to"`
			ID        string `json:"id"`
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Text      struct {
				Body string `json:"body"`
			} `json:"text"`
		} `json:"message_echoes"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := make([]MessageEchoEvent, 0, len(value.MessageEchoes))
	for _, echo := range value.MessageEchoes {
		body := echo.Text.Body
		if body == "" {
			body = fmt.Sprintf("[%s message]", echo.Type)
		}
		items = append(items, MessageEchoEvent{
			PhoneNumberID: value.Metadata.PhoneNumberID,
			From:          echo.From,
			To:            echo.To,
			ID:            echo.ID,
			Timestamp:     echo.Timestamp,
			Type:          echo.Type,
			Body:          body,
		})
	}
	return items, nil
}
