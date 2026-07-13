package inbox

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

type encodedCursor struct {
	PriorityScore int64      `json:"priority_score"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	ID            string     `json:"id"`
}

// EncodeCursor serializes a list cursor to an opaque string.
func EncodeCursor(c *conversation.ListCursor) (string, error) {
	if c == nil {
		return "", nil
	}
	raw, err := json.Marshal(encodedCursor{
		PriorityScore: c.PriorityScore,
		LastMessageAt: c.LastMessageAt,
		ID:            c.ID.String(),
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor parses an opaque cursor string.
func DecodeCursor(raw string) (*conversation.ListCursor, error) {
	raw = trimCursor(raw)
	if raw == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	var enc encodedCursor
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	id, err := uuid.Parse(enc.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor id")
	}
	return &conversation.ListCursor{
		PriorityScore: enc.PriorityScore,
		LastMessageAt: enc.LastMessageAt,
		ID:            id,
	}, nil
}

func trimCursor(raw string) string {
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\n') {
		raw = raw[1:]
	}
	for len(raw) > 0 && (raw[len(raw)-1] == ' ' || raw[len(raw)-1] == '\n') {
		raw = raw[:len(raw)-1]
	}
	return raw
}
