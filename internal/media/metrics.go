package media

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

func (NoopMetrics) IncCacheHit()        {}
func (NoopMetrics) IncCacheMiss()       {}
func (NoopMetrics) IncUpload()          {}
func (NoopMetrics) IncRefresh()         {}
func (NoopMetrics) IncUploadFailure()   {}
func (NoopMetrics) IncDeprecatedLink()  {}

// MemoryMetrics is a simple counter set for tests and local process metrics.
type MemoryMetrics struct {
	CacheHit        int64
	CacheMiss       int64
	Upload          int64
	Refresh         int64
	UploadFailure   int64
	DeprecatedLink  int64
}

func (m *MemoryMetrics) IncCacheHit()       { m.CacheHit++ }
func (m *MemoryMetrics) IncCacheMiss()      { m.CacheMiss++ }
func (m *MemoryMetrics) IncUpload()         { m.Upload++ }
func (m *MemoryMetrics) IncRefresh()        { m.Refresh++ }
func (m *MemoryMetrics) IncUploadFailure()  { m.UploadFailure++ }
func (m *MemoryMetrics) IncDeprecatedLink() { m.DeprecatedLink++ }
