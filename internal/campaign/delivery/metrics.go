package delivery

import "sync"

// Metrics tracks campaign delivery observability counters.
type Metrics struct {
	mu sync.Mutex

	StatusUpdatesTotal  int64
	CounterUpdatesTotal int64
	CompletionTotal     int64
	RecipientFailures   int64
	ReconciliationRepairs int64
}

func (m *Metrics) IncStatusUpdates() {
	m.mu.Lock()
	m.StatusUpdatesTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncCounterUpdates() {
	m.mu.Lock()
	m.CounterUpdatesTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncCompletion() {
	m.mu.Lock()
	m.CompletionTotal++
	m.mu.Unlock()
}

func (m *Metrics) IncRecipientFailures() {
	m.mu.Lock()
	m.RecipientFailures++
	m.mu.Unlock()
}

func (m *Metrics) IncReconciliationRepairs() {
	m.mu.Lock()
	m.ReconciliationRepairs++
	m.mu.Unlock()
}

func (m *Metrics) Snapshot() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]int64{
		"campaign_status_updates_total":  m.StatusUpdatesTotal,
		"campaign_counter_updates_total": m.CounterUpdatesTotal,
		"campaign_completion_total":      m.CompletionTotal,
		"recipient_failures_total":     m.RecipientFailures,
		"reconciliation_repairs_total":   m.ReconciliationRepairs,
	}
}
