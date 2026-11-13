package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestHealthEndpointPostgresAndNATSDown(t *testing.T) {
	checker := NewHealthChecker(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	checkCtx, cancel := context.WithTimeout(req.Context(), time.Second)
	defer cancel()
	status := checker.Check(checkCtx)
	_ = json.NewEncoder(w).Encode(status)

	if status.Healthy {
		t.Fatal("expected unhealthy when postgres and nats are down")
	}
	if status.Postgres || status.NATS {
		t.Fatal("expected postgres and nats false")
	}
}

func TestHealthEndpointHealthyWhenConnected(t *testing.T) {
	// Integration check: needs a NATS server on nats.DefaultURL. Connect without
	// RetryOnFailedConnect so a missing server surfaces as an error (and a skip)
	// instead of a disconnected-but-non-nil connection.
	nc, err := nats.Connect(nats.DefaultURL, nats.Timeout(500*time.Millisecond), nats.MaxReconnects(0))
	if err != nil || !nc.IsConnected() {
		if nc != nil {
			nc.Close()
		}
		t.Skip("nats unavailable for health test")
	}
	defer nc.Close()

	checker := NewHealthChecker(nil, nil, nc)
	status := checker.Check(context.Background())
	if !status.NATS {
		t.Fatal("expected nats healthy")
	}
}
