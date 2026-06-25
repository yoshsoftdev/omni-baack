package worker

import "time"

const (
	DefaultBatchLease       = 10 * time.Minute
	DefaultRateLimitPerSec  = 80
	DefaultMaxSendAttempts  = 3
	DefaultRateLimitBackoff = 2 * time.Second
)

// Config tunes broadcast send worker behavior.
type Config struct {
	BatchLease       time.Duration
	RateLimitPerSec  int
	MaxSendAttempts  int
	RateLimitBackoff time.Duration
	BroadcastStream  string
	WorkerID         string
	RetryDelayFn     func(attempt int) time.Duration
}

func DefaultConfig() Config {
	return Config{
		BatchLease:       DefaultBatchLease,
		RateLimitPerSec:  DefaultRateLimitPerSec,
		MaxSendAttempts:  DefaultMaxSendAttempts,
		RateLimitBackoff: DefaultRateLimitBackoff,
		BroadcastStream:  "BROADCAST",
	}
}

func (c Config) Normalize() Config {
	if c.BatchLease <= 0 {
		c.BatchLease = DefaultBatchLease
	}
	if c.RateLimitPerSec <= 0 {
		c.RateLimitPerSec = DefaultRateLimitPerSec
	}
	if c.MaxSendAttempts <= 0 {
		c.MaxSendAttempts = DefaultMaxSendAttempts
	}
	if c.RateLimitBackoff <= 0 {
		c.RateLimitBackoff = DefaultRateLimitBackoff
	}
	if c.BroadcastStream == "" {
		c.BroadcastStream = "BROADCAST"
	}
	return c
}

// RetryDelay returns backoff before the next attempt (1-based attempt index).
func RetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	default:
		return 10 * time.Minute
	}
}

func (c Config) delayForAttempt(attempt int) time.Duration {
	if c.RetryDelayFn != nil {
		return c.RetryDelayFn(attempt)
	}
	return RetryDelay(attempt)
}
