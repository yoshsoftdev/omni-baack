package media

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

// UploadRetryPolicy controls retries for transient provider upload failures.
type UploadRetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	// IsTransient classifies retryable errors. Nil uses DefaultTransientUploadError.
	IsTransient func(error) bool
}

// DefaultUploadRetryPolicy retries a few times with short backoff for transient failures.
func DefaultUploadRetryPolicy() UploadRetryPolicy {
	return UploadRetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   200 * time.Millisecond,
		IsTransient: DefaultTransientUploadError,
	}
}

// Normalize applies defaults.
func (p UploadRetryPolicy) Normalize() UploadRetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 1
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 100 * time.Millisecond
	}
	if p.IsTransient == nil {
		p.IsTransient = DefaultTransientUploadError
	}
	return p
}

// DefaultTransientUploadError treats network / 5xx / rate-limit style errors as retryable.
func DefaultTransientUploadError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "temporar"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "429"),
		strings.Contains(msg, "rate limit"),
		strings.Contains(msg, "503"),
		strings.Contains(msg, "502"),
		strings.Contains(msg, "504"),
		strings.Contains(msg, "500"):
		return true
	default:
		return false
	}
}

// RefreshWaitPolicy waits for another caller's single-flight refresh to finish.
type RefreshWaitPolicy struct {
	MaxWait  time.Duration
	Interval time.Duration
}

// DefaultRefreshWaitPolicy is used when ClaimRefresh loses the race.
func DefaultRefreshWaitPolicy() RefreshWaitPolicy {
	return RefreshWaitPolicy{
		MaxWait:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}
}

func (p RefreshWaitPolicy) Normalize() RefreshWaitPolicy {
	if p.MaxWait <= 0 {
		p.MaxWait = 5 * time.Second
	}
	if p.Interval <= 0 {
		p.Interval = 100 * time.Millisecond
	}
	return p
}
