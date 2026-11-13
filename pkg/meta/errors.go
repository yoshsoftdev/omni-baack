package meta

import (
	"encoding/json"
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

// MetaCode returns the Graph API error.code from the response body, or 0.
func (e *APIError) MetaCode() int {
	var parsed struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &parsed) != nil {
		return 0
	}
	return parsed.Error.Code
}

// rateLimitCodes are Meta throttling errors. Cloud API returns most of them with
// HTTP 400, not 429, so the status code alone misses them.
var rateLimitCodes = map[int]bool{
	4:      true, // application request limit
	17:     true, // user request limit
	613:    true, // calls within one hour exceeded
	80007:  true, // WhatsApp Business Account rate limit
	130429: true, // Cloud API throughput limit
	131048: true, // spam rate limit (temporary)
	131056: true, // pair rate limit: too many messages to one recipient
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

// IsRateLimited reports throttling: HTTP 429 or a Meta rate-limit error code.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || rateLimitCodes[apiErr.MetaCode()]
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
		return apiErr.StatusCode >= 500 || IsRateLimited(apiErr)
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
