package meta

import (
	"errors"
	"testing"
)

func TestRateLimitDetection(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		limited   bool
		retryable bool
	}{
		{"http 429", NewAPIError(429, `{}`), true, true},
		// Regression: Cloud API sends throughput errors as HTTP 400.
		{"130429 over 400", NewAPIError(400, `{"error":{"message":"(#130429) Rate limit hit","code":130429}}`), true, true},
		{"pair rate limit", NewAPIError(400, `{"error":{"code":131056}}`), true, true},
		{"invalid parameter", NewAPIError(400, `{"error":{"message":"Parameter value character limit exceeded","code":100}}`), false, false},
		{"invalid number", NewAPIError(400, `{"error":{"code":131026}}`), false, false},
		{"non-json body", NewAPIError(400, `rate limit`), false, false},
		{"server error", NewAPIError(503, ``), false, true},
		{"auth", NewAPIError(401, `{"error":{"code":4}}`), true, false},
		{"plain error", errors.New("boom"), false, false},
	}
	for _, c := range cases {
		if got := IsRateLimited(c.err); got != c.limited {
			t.Errorf("%s: IsRateLimited = %v, want %v", c.name, got, c.limited)
		}
		if got := IsRetryable(c.err); got != c.retryable {
			t.Errorf("%s: IsRetryable = %v, want %v", c.name, got, c.retryable)
		}
	}
}
