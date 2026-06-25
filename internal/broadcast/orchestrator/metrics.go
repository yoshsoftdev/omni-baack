package orchestrator

import "sync"

// Metrics tracks orchestrator observability counters.
type Metrics struct {
	mu sync.Mutex

	SchedulerCampaigns   int64
	AudienceSnapshots    int64
	RecipientsCreated    int64
	BatchesCreated       int64
	BatchRecovery        int64
}

func (m *Metrics) IncSchedulerCampaigns() {
	m.mu.Lock()
	m.SchedulerCampaigns++
	m.mu.Unlock()
}

func (m *Metrics) IncAudienceSnapshots() {
	m.mu.Lock()
	m.AudienceSnapshots++
	m.mu.Unlock()
}

func (m *Metrics) AddRecipientsCreated(n int64) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	m.RecipientsCreated += n
	m.mu.Unlock()
}

func (m *Metrics) AddBatchesCreated(n int64) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	m.BatchesCreated += n
	m.mu.Unlock()
}

func (m *Metrics) IncBatchRecovery() {
	m.mu.Lock()
	m.BatchRecovery++
	m.mu.Unlock()
}

func (m *Metrics) Snapshot() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]int64{
		"scheduler_campaigns_total":  m.SchedulerCampaigns,
		"audience_snapshots_total":   m.AudienceSnapshots,
		"recipients_created_total":   m.RecipientsCreated,
		"batches_created_total":      m.BatchesCreated,
		"batch_recovery_total":       m.BatchRecovery,
	}
}
