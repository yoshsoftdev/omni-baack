package eventconsumer

import (
	"sync"
	"time"
)

// Metrics tracks consumer observability counters.
type Metrics struct {
	mu sync.Mutex

	EventsTotal           int64
	FailuresTotal         int64
	RetriesTotal          int64
	DLQTotal              int64
	ConversationsCreated  int64
	MessagesCreated       int64
	StatusUpdatesTotal    int64
}

func (m *Metrics) IncEvents() {
	m.mu.Lock()
	m.EventsTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncFailures() {
	m.mu.Lock()
	m.FailuresTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncRetries() {
	m.mu.Lock()
	m.RetriesTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncDLQ() {
	m.mu.Lock()
	m.DLQTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncConversationsCreated() {
	m.mu.Lock()
	m.ConversationsCreated++
	m.mu.Unlock()
}

func (m *Metrics) IncMessagesCreated() {
	m.mu.Lock()
	m.MessagesCreated++
	m.mu.Unlock()
}

func (m *Metrics) IncStatusUpdates() {
	m.mu.Lock()
	m.StatusUpdatesTotal++
	m.mu.Unlock()
}

// Snapshot returns a copy of current metrics.
func (m *Metrics) Snapshot() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]int64{
		"consumer_events_total":          m.EventsTotal,
		"consumer_failures_total":        m.FailuresTotal,
		"consumer_retries_total":         m.RetriesTotal,
		"consumer_dlq_total":             m.DLQTotal,
		"conversation_created_total":     m.ConversationsCreated,
		"messages_created_total":         m.MessagesCreated,
		"status_updates_total":           m.StatusUpdatesTotal,
		"consumer_snapshot_at_unix":      time.Now().Unix(),
	}
}
