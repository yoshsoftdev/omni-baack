package media

import "sync/atomic"

// Metrics observes Media Manager lifecycle events.
// Implementations may be no-op, in-memory, or Prometheus exporters.
type Metrics interface {
	IncCacheHit()
	IncCacheMiss()
	IncUpload()
	IncRefresh()
	IncUploadFailure()
	IncDeprecatedLink()
}

// NoopMetrics discards all observations.
type NoopMetrics struct{}

func (NoopMetrics) IncCacheHit()       {}
func (NoopMetrics) IncCacheMiss()      {}
func (NoopMetrics) IncUpload()         {}
func (NoopMetrics) IncRefresh()        {}
func (NoopMetrics) IncUploadFailure()  {}
func (NoopMetrics) IncDeprecatedLink() {}

// MemoryMetrics is a simple counter set for tests and local process metrics.
// It is shared by concurrent senders (api, broadcast-worker), so every increment
// is atomic. Read fields with Snapshot (or atomic.LoadInt64) while it is in use.
type MemoryMetrics struct {
	CacheHit       int64
	CacheMiss      int64
	Upload         int64
	Refresh        int64
	UploadFailure  int64
	DeprecatedLink int64
}

func (m *MemoryMetrics) IncCacheHit()       { atomic.AddInt64(&m.CacheHit, 1) }
func (m *MemoryMetrics) IncCacheMiss()      { atomic.AddInt64(&m.CacheMiss, 1) }
func (m *MemoryMetrics) IncUpload()         { atomic.AddInt64(&m.Upload, 1) }
func (m *MemoryMetrics) IncRefresh()        { atomic.AddInt64(&m.Refresh, 1) }
func (m *MemoryMetrics) IncUploadFailure()  { atomic.AddInt64(&m.UploadFailure, 1) }
func (m *MemoryMetrics) IncDeprecatedLink() { atomic.AddInt64(&m.DeprecatedLink, 1) }

// Snapshot returns a consistent-per-field copy of the counters, safe during concurrent use.
func (m *MemoryMetrics) Snapshot() MemoryMetrics {
	return MemoryMetrics{
		CacheHit:       atomic.LoadInt64(&m.CacheHit),
		CacheMiss:      atomic.LoadInt64(&m.CacheMiss),
		Upload:         atomic.LoadInt64(&m.Upload),
		Refresh:        atomic.LoadInt64(&m.Refresh),
		UploadFailure:  atomic.LoadInt64(&m.UploadFailure),
		DeprecatedLink: atomic.LoadInt64(&m.DeprecatedLink),
	}
}
