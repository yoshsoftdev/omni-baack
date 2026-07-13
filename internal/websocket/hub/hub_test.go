package hub

import (
	"testing"

	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

func TestHub_RegisterUnregister(t *testing.T) {
	h := New(logger.New("test"), nil)
	client := &Client{
		TenantID: "tenant-1",
		UserID:   "user-1",
		send:     make(chan []byte, 1),
	}

	h.Register(client)
	h.mu.RLock()
	count := len(h.rooms[tenantRoom("tenant-1")])
	h.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 client, got %d", count)
	}

	h.Unregister(client)
	h.mu.RLock()
	count = len(h.rooms[tenantRoom("tenant-1")])
	h.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 clients after unregister, got %d", count)
	}
}

func TestHub_BroadcastToTenant(t *testing.T) {
	h := New(logger.New("test"), nil)
	client := &Client{
		TenantID: "tenant-1",
		UserID:   "user-1",
		send:     make(chan []byte, 1),
	}
	h.Register(client)

	payload := []byte(`{"event":"ping"}`)
	h.BroadcastToTenant("tenant-1", payload)

	select {
	case got := <-client.send:
		if string(got) != string(payload) {
			t.Errorf("payload = %s", got)
		}
	default:
		t.Fatal("expected message in send channel")
	}
}

func TestHub_HasUserConnection(t *testing.T) {
	h := New(logger.New("test"), nil)
	client := &Client{
		TenantID: "tenant-1",
		UserID:   "user-1",
		send:     make(chan []byte, 1),
	}
	h.Register(client)

	if !h.HasUserConnection("tenant-1", "user-1") {
		t.Fatal("expected user connection")
	}
	if h.HasUserConnection("tenant-1", "user-2") {
		t.Fatal("did not expect other user connection")
	}

	h.Unregister(client)
	if h.HasUserConnection("tenant-1", "user-1") {
		t.Fatal("expected no user connection after unregister")
	}
}
