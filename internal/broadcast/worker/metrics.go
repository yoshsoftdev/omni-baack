package worker

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Metrics tracks send worker observability counters.
type Metrics struct {
	mu sync.Mutex

	SendAttempts             int64
	SendSuccess              int64
	SendFailed               int64
	SendRetry                int64
	MetaRateLimit            int64
	BatchProcessingMillis    int64
	TemplateValidationFailed int64
	TemplateRenderSuccess    int64
	TemplateRenderFailed     int64
	RateLimitFallback        int64
	MediaIDSend              int64
	MediaLinkSend            int64
	RecipientRecovery        int64
	WorkerRestarts           int64
	WorkerBatchesProcessing  int64
	WorkerBatchesCompleted   int64
	MetaSendDurationMillis   int64

	byChannel map[uuid.UUID]*channelMetrics
}

type channelMetrics struct {
	attempts                 int64
	success                  int64
	failed                   int64
	retry                    int64
	rateLimit                int64
	templateValidationFailed int64
	templateRenderSuccess    int64
	templateRenderFailed     int64
	rateLimitFallback        int64
	mediaIDSend              int64
	mediaLinkSend            int64
}

func NewMetrics() *Metrics {
	return &Metrics{byChannel: make(map[uuid.UUID]*channelMetrics)}
}

func (m *Metrics) channel(channelID uuid.UUID) *channelMetrics {
	if m.byChannel == nil {
		m.byChannel = make(map[uuid.UUID]*channelMetrics)
	}
	cm, ok := m.byChannel[channelID]
	if !ok {
		cm = &channelMetrics{}
		m.byChannel[channelID] = cm
	}
	return cm
}

func (m *Metrics) IncSendAttempt(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendAttempts++
	m.channel(channelID).attempts++
}

func (m *Metrics) IncSendSuccess(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendSuccess++
	m.channel(channelID).success++
}

func (m *Metrics) IncSendFailed(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendFailed++
	m.channel(channelID).failed++
}

func (m *Metrics) IncSendRetry(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendRetry++
	m.channel(channelID).retry++
}

func (m *Metrics) IncMetaRateLimit(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MetaRateLimit++
	m.channel(channelID).rateLimit++
}

func (m *Metrics) IncTemplateValidationFailed(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TemplateValidationFailed++
	m.channel(channelID).templateValidationFailed++
}

func (m *Metrics) IncTemplateRenderSuccess(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TemplateRenderSuccess++
	m.channel(channelID).templateRenderSuccess++
}

func (m *Metrics) IncTemplateRenderFailed(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TemplateRenderFailed++
	m.channel(channelID).templateRenderFailed++
}

func (m *Metrics) IncRateLimitFallback(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RateLimitFallback++
	m.channel(channelID).rateLimitFallback++
}

func (m *Metrics) IncMediaIDSend(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MediaIDSend++
	m.channel(channelID).mediaIDSend++
}

func (m *Metrics) IncMediaLinkSend(channelID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MediaLinkSend++
	m.channel(channelID).mediaLinkSend++
}

func (m *Metrics) AddRecipientRecovery(n int64) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecipientRecovery += n
}

func (m *Metrics) IncWorkerRestart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WorkerRestarts++
}

func (m *Metrics) IncWorkerBatchesProcessing() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WorkerBatchesProcessing++
}

func (m *Metrics) IncWorkerBatchesCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WorkerBatchesCompleted++
}

func (m *Metrics) ObserveMetaSendDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MetaSendDurationMillis += d.Milliseconds()
}

func (m *Metrics) ObserveBatchProcessing(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BatchProcessingMillis += d.Milliseconds()
}

func (m *Metrics) Snapshot() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]int64{
		"send_attempts_total":              m.SendAttempts,
		"send_success_total":               m.SendSuccess,
		"send_failed_total":                m.SendFailed,
		"send_retry_total":                 m.SendRetry,
		"meta_rate_limit_total":            m.MetaRateLimit,
		"batch_processing_seconds":         m.BatchProcessingMillis / 1000,
		"template_validation_failed_total": m.TemplateValidationFailed,
		"template_render_success_total":    m.TemplateRenderSuccess,
		"template_render_failed_total":     m.TemplateRenderFailed,
		"rate_limit_fallback_total":        m.RateLimitFallback,
		"media_id_sends_total":             m.MediaIDSend,
		"media_link_sends_total":           m.MediaLinkSend,
		"recipient_recovery_total":         m.RecipientRecovery,
		"worker_restarts_total":            m.WorkerRestarts,
		"worker_batches_processing":        m.WorkerBatchesProcessing,
		"worker_batches_completed":           m.WorkerBatchesCompleted,
		"meta_send_duration_ms":            m.MetaSendDurationMillis,
	}
}
