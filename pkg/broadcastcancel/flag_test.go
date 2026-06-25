package broadcastcancel

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestKeyFor(t *testing.T) {
	id := uuid.New()
	key := KeyFor(id)
	if !strings.Contains(key, id.String()) {
		t.Fatalf("expected campaign id in key, got %s", key)
	}
	if !strings.HasPrefix(key, KeyPrefix) {
		t.Fatalf("expected prefix %s, got %s", KeyPrefix, key)
	}
}
