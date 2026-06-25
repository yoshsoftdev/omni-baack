package meta_test

import (
	"context"
	"errors"
	"testing"
	"time"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestRetrySuccessAfterTransientFailure(t *testing.T) {
	old := metapkg.RetryDelays
	metapkg.RetryDelays = []time.Duration{time.Millisecond}
	defer func() { metapkg.RetryDelays = old }()

	attempts := 0
	err := metapkg.Retry(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 2 {
			return metapkg.NewAPIError(500, "server error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestRetryFailsOnInvalidToken(t *testing.T) {
	old := metapkg.RetryDelays
	metapkg.RetryDelays = []time.Duration{time.Millisecond}
	defer func() { metapkg.RetryDelays = old }()

	attempts := 0
	err := metapkg.Retry(context.Background(), func(context.Context) error {
		attempts++
		return metapkg.NewAPIError(401, "invalid token")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d", attempts)
	}
	if !metapkg.IsInvalidToken(err) {
		t.Fatal("expected invalid token")
	}
}

func TestRetryRateLimited(t *testing.T) {
	old := metapkg.RetryDelays
	metapkg.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { metapkg.RetryDelays = old }()

	attempts := 0
	err := metapkg.Retry(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return metapkg.NewAPIError(429, "rate limited")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestIsRetryableNetworkError(t *testing.T) {
	if !metapkg.IsRetryable(errors.New("connection reset by peer")) {
		t.Fatal("expected retryable network error")
	}
}
