package meta

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// APIError represents a Meta Graph API HTTP error response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("meta api error %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

func NewAPIError(statusCode int, body string) *APIError {
	return &APIError{StatusCode: statusCode, Body: body}
}

// IsInvalidToken reports auth failures that must not be retried.
func IsInvalidToken(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}
	return false
}

// IsMissingResource reports 404-style failures that must not be retried.
func IsMissingResource(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}

// IsRateLimited reports HTTP 429 responses.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429
	}
	return false
}

// IsAlreadySubscribed reports Meta responses indicating the app is already subscribed.
func IsAlreadySubscribed(statusCode int, body string) bool {
	if statusCode < 400 {
		return false
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "already subscribed") ||
		strings.Contains(lower, "already installed") ||
		strings.Contains(lower, "duplicate subscription")
}

// IsSubscribeAlreadyDone reports whether a subscribe error can be treated as success.
func IsSubscribeAlreadyDone(err error) bool {
	if err == nil {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return IsAlreadySubscribed(apiErr.StatusCode, apiErr.Body)
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "already subscribed") ||
		strings.Contains(lower, "already installed") ||
		strings.Contains(lower, "duplicate subscription")
}

// IsRetryable reports transient failures eligible for retry/backoff.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if IsInvalidToken(err) || IsMissingResource(err) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500 || apiErr.StatusCode == 429
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "temporary failure")
}
