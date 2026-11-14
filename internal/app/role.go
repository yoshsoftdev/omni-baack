package app

// Role selects which part of the backend a process runs (6.3). Each role is its
// own binary so it can be scaled and restarted independently.
type Role string

const (
	// RoleAPI serves HTTP and WebSocket. Safe to run many replicas.
	RoleAPI Role = "api"
	// RoleEventConsumer consumes webhook events from NATS (meta.>). Replicas share the durable consumer.
	RoleEventConsumer Role = "event-consumer"
	// RoleScheduler runs periodic jobs; each job holds a Postgres advisory lock, so extra replicas only stand by.
	RoleScheduler Role = "scheduler"
)

// defaultMetricsAddr keeps roles on distinct ports when they share a host (METRICS_ADDR overrides).
func (r Role) defaultMetricsAddr() string {
	switch r {
	case RoleEventConsumer:
		return "127.0.0.1:9106"
	case RoleScheduler:
		return "127.0.0.1:9107"
	default:
		return "127.0.0.1:9101"
	}
}
