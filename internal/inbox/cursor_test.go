package inbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

func TestCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := &conversation.ListCursor{
		PriorityScore: 250,
		LastMessageAt: &now,
		ID:            uuid.New(),
	}
	encoded, err := EncodeCursor(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PriorityScore != original.PriorityScore {
		t.Fatalf("score=%d", decoded.PriorityScore)
	}
	if decoded.ID != original.ID {
		t.Fatalf("id mismatch")
	}
}

func TestDecodeCursor_Empty(t *testing.T) {
	c, err := DecodeCursor("")
	if err != nil || c != nil {
		t.Fatalf("got %v err %v", c, err)
	}
}
