package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/redis/go-redis/v9"
)

// HealthStatus is returned by GET /health.
type HealthStatus struct {
	Healthy  bool `json:"healthy"`
	NATS     bool `json:"nats"`
	Postgres bool `json:"postgres"`
	Redis    bool `json:"redis"`
}

// HealthChecker probes worker dependencies.
type HealthChecker struct {
	db    *database.DB
	redis *redis.Client
	nc    *nats.Conn
}

func NewHealthChecker(db *database.DB, redis *redis.Client, nc *nats.Conn) *HealthChecker {
	return &HealthChecker{db: db, redis: redis, nc: nc}
}

func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{}
	if h.db != nil {
		if err := h.db.Pool.Ping(ctx); err == nil {
			status.Postgres = true
		}
	}
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err == nil {
			status.Redis = true
		}
	}
	if h.nc != nil && h.nc.IsConnected() {
		status.NATS = true
	}
	status.Healthy = status.Postgres && status.NATS
	return status
}

// ServeHealth starts a lightweight HTTP health endpoint.
func ServeHealth(ctx context.Context, addr string, checker *HealthChecker) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		status := checker.Check(checkCtx)
		w.Header().Set("Content-Type", "application/json")
		if !status.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(status)
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		_ = srv.ListenAndServe()
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv
}
